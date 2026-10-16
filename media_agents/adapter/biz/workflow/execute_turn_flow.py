"""Execute Turn workflow: durable state around transaction-free Runtime calls."""

from __future__ import annotations

import asyncio
import logging
from collections.abc import Awaitable, Callable
from dataclasses import dataclass, field
from typing import Protocol

from adapter.biz.domain.acp_adapter.errors import AuthenticationRequired
from adapter.biz.domain.runtime_session.entity import (
    AgentRequest,
    RuntimeSession,
    Turn,
    TurnStatus,
    UniversalEvent,
)
from adapter.biz.domain.runtime_session.errors import (
    ExecutionCredentialsDeferred,
    ExecutionCredentialsRejected,
    InvalidTurn,
    ModelAuthenticationRequired,
    StaleExecutionOwner,
)
from adapter.biz.service.runtime_session.command import SubmitTurn
from adapter.biz.service.runtime_session.service import (
    AgentRequestChange,
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
    codex_auth_json: str | None = field(default=None, repr=False)
    model_connection_id: str = ""
    model_connection_version: int = 0
    model_protocol: str = ""
    model_name: str = ""
    model_base_url: str = ""
    mcp_url: str | None = field(default=None, repr=False)
    mcp_headers: dict[str, str] = field(default_factory=dict, repr=False)


@dataclass(frozen=True, slots=True)
class RuntimeResult:
    status: TurnStatus
    error_class: str | None = None


AgentRequestHandler = Callable[[str, dict[str, object]], Awaitable[dict[str, object]]]


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
        request_handler: AgentRequestHandler,
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

    async def open_agent_request(
        self, session_id: str, turn_id: str, request_type: str, payload: dict[str, object]
    ) -> AgentRequestChange: ...

    async def running_turns(self) -> list[Turn]: ...

    async def get_turn(self, turn_id: str) -> Turn: ...

    async def resolve_agent_request(
        self, session_id: str, turn_id: str, request_id: str, response: dict[str, object]
    ) -> AgentRequestChange: ...


class AdoptionUseCases(Protocol):
    async def adopt(
        self, session_id: str, *, resume: bool = False, operation_id: str = ""
    ) -> TurnSubmission | None: ...


class ExecuteTurnFlow:
    def __init__(
        self,
        service: RuntimeSessionUseCases,
        runtime: TurnRuntime,
        sink_factory: Callable[[str, str], TurnEventSink],
        adoption_service: AdoptionUseCases | None = None,
        credential_resolver: Callable[[str, str, int], Awaitable[RuntimeCredentials]] | None = None,
    ) -> None:
        self._service = service
        self._runtime = runtime
        self._sink_factory = sink_factory
        self._credential_resolver = credential_resolver
        self._adoption_service = adoption_service
        self._tasks: dict[str, asyncio.Task[None]] = {}
        self._scheduled_turns: set[str] = set()
        self._credentials: dict[str, RuntimeCredentials] = {}
        self._turn_sessions: dict[str, str] = {}
        self._interrupt_requested: set[str] = set()
        self._session_locks: dict[str, asyncio.Lock] = {}
        self._request_waiters: dict[str, asyncio.Future[dict[str, object]]] = {}
        self._early_responses: dict[str, dict[str, object]] = {}

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

    async def resolve_request(
        self, session_id: str, turn_id: str, request_id: str, response: dict[str, object]
    ) -> AgentRequest:
        change = await self._service.resolve_agent_request(
            session_id, turn_id, request_id, response
        )
        if change.event is not None:
            await self._sink_factory(session_id, turn_id).publish_committed(change.event)
        waiter = self._request_waiters.get(request_id)
        if waiter is not None and not waiter.done():
            waiter.set_result(change.request.response or response)
        else:
            self._early_responses[request_id] = change.request.response or response
        return change.request

    async def wait(self, session_id: str) -> None:
        task = self._tasks.get(session_id)
        if task is not None:
            await asyncio.shield(task)

    async def recover(self) -> list[Turn]:
        recovered: list[Turn] = []
        for turn in await self._service.running_turns():
            try:
                async with self._session_lock(turn.session_id):
                    finalized = await self._service.finalize_turn(
                        turn.session_id,
                        turn.turn_id,
                        TurnStatus.FAILED,
                        error_class="recovery_outcome_unknown",
                    )
            except (InvalidTurn, StaleExecutionOwner):
                current = await self._service.get_turn(turn.turn_id)
                if current.status is TurnStatus.RUNNING:
                    raise
                continue
            sink = self._sink_factory(turn.session_id, turn.turn_id)
            for event in finalized.events:
                await sink.publish_committed(event)
            recovered.append(finalized.terminal_turn)
        return recovered

    async def publish_transferred(self, events: tuple[UniversalEvent, ...]) -> None:
        for event in events:
            await self._sink_factory(event.session_id, event.turn_id or "").publish_committed(event)

    def local_idle(self) -> bool:
        return not any(not task.done() for task in self._tasks.values())

    async def adopt(self, session_id: str, *, resume: bool = False, operation_id: str = "") -> None:
        async with self._session_lock(session_id):
            if self._adoption_service is None:
                raise InvalidTurn("adoption_unavailable")
            submission = await self._adoption_service.adopt(
                session_id, resume=resume, operation_id=operation_id
            )
            if submission is not None:
                for event in submission.events:
                    await self._sink_factory(session_id, submission.turn.turn_id).publish_committed(
                        event
                    )
                self._schedule(session_id, submission.turn.turn_id)

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
        for waiter in self._request_waiters.values():
            waiter.cancel()
        self._request_waiters.clear()
        self._early_responses.clear()

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
            active_turn_id = current_turn_id
            try:
                if self._credential_resolver is not None:
                    credentials = await self._fresh_credentials(
                        session_id, current_turn_id, execution.session.execution_epoch
                    )
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
                        preparation,
                        session,
                        execution.turn,
                        credentials,
                        sink.accept,
                        lambda request_type, payload: self._await_agent_request(
                            session_id, active_turn_id, sink, request_type, payload
                        ),
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
            except _InterruptedBeforePreparation:
                result = RuntimeResult(TurnStatus.INTERRUPTED)
            except StaleExecutionOwner:
                await sink.discard()
                return
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

    async def _await_agent_request(
        self,
        session_id: str,
        turn_id: str,
        sink: TurnEventSink,
        request_type: str,
        payload: dict[str, object],
    ) -> dict[str, object]:
        change = await self._service.open_agent_request(session_id, turn_id, request_type, payload)
        request = change.request
        loop = asyncio.get_running_loop()
        waiter: asyncio.Future[dict[str, object]] = loop.create_future()
        self._request_waiters[request.request_id] = waiter
        if change.event is None:
            raise InvalidTurn("new Agent Request did not produce a durable event")
        await sink.publish_committed(change.event)
        early = self._early_responses.pop(request.request_id, None)
        if early is not None and not waiter.done():
            waiter.set_result(early)
        try:
            return await waiter
        finally:
            self._request_waiters.pop(request.request_id, None)

    async def _fresh_credentials(
        self, session_id: str, turn_id: str, epoch: int
    ) -> RuntimeCredentials:
        if self._credential_resolver is None:
            raise ExecutionCredentialsRejected()
        delay = 1
        while True:
            if turn_id in self._interrupt_requested:
                raise _InterruptedBeforePreparation()
            await self._service.execution(session_id, turn_id)
            try:
                return await self._credential_resolver(session_id, turn_id, epoch)
            except ExecutionCredentialsDeferred:
                for _ in range(delay * 10):
                    if turn_id in self._interrupt_requested:
                        raise _InterruptedBeforePreparation()
                    await asyncio.sleep(0.1)
                delay = min(delay * 2, 30)

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
    if isinstance(error, (AuthenticationRequired, ModelAuthenticationRequired)):
        return "model_authentication_required"
    if isinstance(error, ExecutionCredentialsRejected):
        return "execution_credentials_rejected"
    if isinstance(error, InvalidTurn):
        return "invalid_turn_input"
    if isinstance(error, TimeoutError):
        return "runtime_timeout"
    return "runtime_error"


class _InterruptedBeforePreparation(Exception):
    """The queued claim was interrupted before starting native execution."""
