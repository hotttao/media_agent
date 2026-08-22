"""Execute Turn workflow: durable state around transaction-free Runtime calls."""

from __future__ import annotations

import asyncio
import logging
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Protocol

from adapter.biz.domain.runtime_session.entity import RuntimeSession, Turn, TurnStatus
from adapter.biz.domain.runtime_session.errors import InvalidTurn
from adapter.biz.service.runtime_session.command import SubmitTurn
from adapter.biz.service.runtime_session.service import (
    TurnExecution,
    TurnFinalization,
    TurnSubmission,
)
from adapter.biz.workflow.turn_event_sink import RuntimeEventHandler, TurnEventSink

logger = logging.getLogger(__name__)


@dataclass(frozen=True, slots=True)
class RuntimePreparation:
    handle: str
    is_new: bool


@dataclass(frozen=True, slots=True)
class RuntimeCredentials:
    api_key: str | None = field(default=None, repr=False)
    session_token: str | None = field(default=None, repr=False)


@dataclass(frozen=True, slots=True)
class RuntimeResult:
    status: TurnStatus
    error_class: str | None = None


class TurnRuntime(Protocol):
    async def prepare(
        self, session: RuntimeSession, turn: Turn, credentials: RuntimeCredentials
    ) -> RuntimePreparation: ...

    async def execute(
        self,
        preparation: RuntimePreparation,
        session: RuntimeSession,
        turn: Turn,
        credentials: RuntimeCredentials,
        event_handler: RuntimeEventHandler,
    ) -> RuntimeResult: ...

    async def interrupt(self, handle: str) -> bool: ...


class RuntimeSessionUseCases(Protocol):
    async def submit_turn_details(self, command: SubmitTurn) -> TurnSubmission: ...

    async def execution(self, session_id: str, turn_id: str) -> TurnExecution: ...

    async def attach_runtime(
        self, session_id: str, turn_id: str, handle: str
    ) -> RuntimeSession: ...

    async def finalize_turn(
        self,
        session_id: str,
        turn_id: str,
        status: TurnStatus,
        *,
        error_class: str | None = None,
        terminal_payload: dict[str, object] | None = None,
    ) -> TurnFinalization: ...


class ExecuteTurnFlow:
    def __init__(
        self,
        service: RuntimeSessionUseCases,
        runtime: TurnRuntime,
        sink_factory: Callable[[str, str], TurnEventSink],
    ) -> None:
        self._service = service
        self._runtime = runtime
        self._sink_factory = sink_factory
        self._tasks: dict[str, asyncio.Task[None]] = {}
        self._scheduled_turns: set[str] = set()
        self._credentials: dict[str, RuntimeCredentials] = {}
        self._turn_sessions: dict[str, str] = {}
        self._interrupt_requested: set[str] = set()
        self._session_locks: dict[str, asyncio.Lock] = {}

    async def submit(
        self, command: SubmitTurn, credentials: RuntimeCredentials | None = None
    ) -> Turn:
        async with self._session_lock(command.session_id):
            submission = await self._service.submit_turn_details(command)
            turn = submission.turn
            publisher = self._sink_factory(turn.session_id, turn.turn_id)
            for event in submission.events:
                await publisher.publish_committed(event)
            self._turn_sessions[turn.turn_id] = turn.session_id
            if credentials is not None and (credentials.api_key or credentials.session_token):
                self._credentials[turn.turn_id] = credentials
            if turn.status is TurnStatus.RUNNING:
                self._schedule(turn.session_id, turn.turn_id)
        return turn

    async def interrupt(self, session_id: str, turn_id: str) -> bool:
        execution = await self._service.execution(session_id, turn_id)
        self._interrupt_requested.add(turn_id)
        handle = execution.session.runtime_session_id
        if handle is None:
            return True
        await self._send_interrupt(handle)
        return True

    async def wait(self, session_id: str) -> None:
        task = self._tasks.get(session_id)
        if task is not None:
            await asyncio.shield(task)

    async def close(self) -> None:
        tasks = list(self._tasks.values())
        for task in tasks:
            task.cancel()
        if tasks:
            await asyncio.gather(*tasks, return_exceptions=True)
        self._credentials.clear()
        self._turn_sessions.clear()
        self._interrupt_requested.clear()
        self._session_locks.clear()
        self._scheduled_turns.clear()

    def _schedule(self, session_id: str, turn_id: str) -> None:
        if turn_id in self._scheduled_turns:
            return
        task = self._tasks.get(session_id)
        if task is not None and not task.done():
            self._scheduled_turns.add(turn_id)
            task.add_done_callback(lambda _completed: self._schedule_after(session_id, turn_id))
            return
        self._scheduled_turns.add(turn_id)
        task = asyncio.create_task(self._drain(session_id, turn_id))
        self._tasks[session_id] = task
        task.add_done_callback(lambda completed: self._finished(session_id, completed))

    def _finished(self, session_id: str, task: asyncio.Task[None]) -> None:
        if self._tasks.get(session_id) is task:
            self._tasks.pop(session_id, None)
        try:
            task.result()
        except asyncio.CancelledError:
            pass
        except Exception:
            logger.exception("Execute Turn flow failed session=%s", session_id)

    def _schedule_after(self, session_id: str, turn_id: str) -> None:
        self._scheduled_turns.discard(turn_id)
        self._schedule(session_id, turn_id)

    async def _drain(self, session_id: str, turn_id: str) -> None:
        current_turn_id: str | None = turn_id
        while current_turn_id is not None:
            self._scheduled_turns.add(current_turn_id)
            execution = await self._service.execution(session_id, current_turn_id)
            credentials = self._credentials.get(current_turn_id, RuntimeCredentials())
            sink = self._sink_factory(session_id, current_turn_id)
            try:
                preparation = await self._runtime.prepare(
                    execution.session, execution.turn, credentials
                )
                session = execution.session
                if session.runtime_session_id is None:
                    session = await self._service.attach_runtime(
                        session_id, current_turn_id, preparation.handle
                    )
                elif session.runtime_session_id != preparation.handle:
                    raise InvalidTurn("Runtime returned a different persisted handle")
                runtime_task = asyncio.create_task(
                    self._runtime.execute(
                        preparation, session, execution.turn, credentials, sink.accept
                    )
                )
                await asyncio.sleep(0)
                if current_turn_id in self._interrupt_requested:
                    await self._send_interrupt(preparation.handle, runtime_task)
                result = await runtime_task
                if result.status not in {
                    TurnStatus.COMPLETED,
                    TurnStatus.FAILED,
                    TurnStatus.INTERRUPTED,
                }:
                    raise InvalidTurn("Runtime returned a non-terminal status")
                await sink.flush()
            except asyncio.CancelledError:
                await sink.discard()
                raise
            except Exception as error:
                result = RuntimeResult(TurnStatus.FAILED, _stable_error_class(error))
            async with self._session_lock(session_id):
                finalized = await self._service.finalize_turn(
                    session_id,
                    current_turn_id,
                    result.status,
                    error_class=result.error_class,
                    terminal_payload=sink.terminal_payload,
                )
            for event in finalized.events:
                await sink.publish_committed(event)
            current_turn_id = (
                finalized.next_turn.turn_id if finalized.next_turn is not None else None
            )
            self._scheduled_turns.discard(finalized.terminal_turn.turn_id)
            self._credentials.pop(finalized.terminal_turn.turn_id, None)
            self._turn_sessions.pop(finalized.terminal_turn.turn_id, None)
            self._interrupt_requested.discard(finalized.terminal_turn.turn_id)
            if result.status is not TurnStatus.COMPLETED:
                self._clear_session_credentials(session_id)

    async def _send_interrupt(
        self, handle: str, runtime_task: asyncio.Task[RuntimeResult] | None = None
    ) -> bool:
        for _ in range(100):
            if await self._runtime.interrupt(handle):
                return True
            if runtime_task is not None and runtime_task.done():
                return False
            await asyncio.sleep(0.01)
        return False

    def _clear_session_credentials(self, session_id: str) -> None:
        turn_ids = [
            turn_id
            for turn_id, owner_session_id in self._turn_sessions.items()
            if owner_session_id == session_id
        ]
        for turn_id in turn_ids:
            self._credentials.pop(turn_id, None)
            self._turn_sessions.pop(turn_id, None)

    def _session_lock(self, session_id: str) -> asyncio.Lock:
        normalized = session_id.strip()
        lock = self._session_locks.get(normalized)
        if lock is None:
            lock = asyncio.Lock()
            self._session_locks[normalized] = lock
        return lock


def _stable_error_class(error: Exception) -> str:
    if isinstance(error, InvalidTurn):
        return "invalid_turn_input"
    if isinstance(error, TimeoutError):
        return "runtime_timeout"
    return "runtime_error"
