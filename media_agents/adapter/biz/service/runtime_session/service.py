"""Single-domain Runtime Session use cases."""

from __future__ import annotations

from collections.abc import Awaitable, Callable
from dataclasses import dataclass, replace
from datetime import UTC, datetime
from typing import Protocol, TypeVar
from uuid import uuid4

from adapter.biz.domain.runtime_session.entity import (
    ChatStatus,
    Message,
    RuntimeSession,
    SessionStatus,
    Turn,
    TurnStatus,
    UniversalEvent,
)
from adapter.biz.domain.runtime_session.errors import (
    ContextVersionConflict,
    InvalidTurn,
    RuntimeSessionAlreadyExists,
    RuntimeSessionNotFound,
)
from adapter.biz.domain.runtime_session.repository import RuntimeSessionRepository
from adapter.biz.domain.runtime_session.runtime_event import RuntimeEventEnvelope
from adapter.biz.service.runtime_session.command import RegisterRuntimeSession, SubmitTurn

T = TypeVar("T")


class TransactionRunner(Protocol):
    async def run_in_tx(self, operation: Callable[[], Awaitable[T]]) -> T: ...


@dataclass(frozen=True, slots=True)
class TurnExecution:
    session: RuntimeSession
    turn: Turn


@dataclass(frozen=True, slots=True)
class TurnFinalization:
    terminal_turn: Turn
    next_turn: Turn | None
    events: tuple[UniversalEvent, ...] = ()


@dataclass(frozen=True, slots=True)
class TurnSubmission:
    turn: Turn
    events: tuple[UniversalEvent, ...]


class RuntimeSessionService:
    def __init__(self, tx: TransactionRunner, repository: RuntimeSessionRepository) -> None:
        self._tx = tx
        self._repository = repository

    async def register(self, command: RegisterRuntimeSession) -> RuntimeSession:
        candidate = RuntimeSession.register(
            session_id=command.session_id,
            context_version=command.context_version,
            context_ref=command.context_ref,
        )

        async def operation() -> RuntimeSession:
            existing = await self._repository.get(candidate.session_id, for_update=True)
            if existing is not None:
                if existing.context_version != command.context_version:
                    raise ContextVersionConflict(
                        existing.session_id,
                        existing.context_version,
                        command.context_version,
                    )
                return existing
            await self._repository.add(candidate)
            return candidate

        try:
            return await self._tx.run_in_tx(operation)
        except RuntimeSessionAlreadyExists:
            existing = await self._tx.run_in_tx(lambda: self._repository.get(candidate.session_id))
            if existing is None:
                raise
            if existing.context_version != candidate.context_version:
                raise ContextVersionConflict(
                    existing.session_id,
                    existing.context_version,
                    candidate.context_version,
                ) from None
            return existing

    async def get(self, session_id: str) -> RuntimeSession:
        session = await self._tx.run_in_tx(lambda: self._repository.get(session_id.strip()))
        if session is None:
            raise RuntimeSessionNotFound(session_id)
        return session

    async def submit_turn(self, command: SubmitTurn) -> Turn:
        return (await self.submit_turn_details(command)).turn

    async def submit_turn_details(self, command: SubmitTurn) -> TurnSubmission:
        session_id = command.session_id.strip()
        key = command.idempotency_key.strip()
        if not key:
            raise InvalidTurn("idempotency_key is required")
        if not command.input:
            raise InvalidTurn("input is required")
        if _contains_secret(command.input):
            raise InvalidTurn("input must not contain runtime credentials")

        async def operation() -> TurnSubmission:
            session = await self._repository.get(session_id, for_update=True)
            if session is None:
                raise RuntimeSessionNotFound(session_id)
            existing = await self._repository.get_turn_by_idempotency_key(session_id, key)
            if existing is not None:
                return TurnSubmission(existing, ())
            if session.context_version != command.context_version:
                raise ContextVersionConflict(
                    session_id, session.context_version, command.context_version
                )
            if session.status is not SessionStatus.ACTIVE:
                raise InvalidTurn(f"runtime session is not active: {session.status.value}")

            now = datetime.now(UTC)
            turn_id = str(uuid4())
            turn = Turn(
                turn_id=turn_id,
                session_id=session_id,
                idempotency_key=key,
                enqueue_sequence=session.next_turn_sequence,
                status=TurnStatus.QUEUED,
                input=command.input,
                context_version=session.context_version,
                created_at=now,
            )
            session, message = session.new_message(
                turn_id=turn_id,
                role="user",
                content_ref=command.input,
                now=now,
            )
            if session.active_turn_id is None:
                turn = replace(turn, status=TurnStatus.RUNNING, started_at=now)
                session = replace(
                    session,
                    active_turn_id=turn_id,
                    chat_status=ChatStatus.AGENT,
                    next_turn_sequence=session.next_turn_sequence + 1,
                    updated_at=now,
                )
            else:
                session = replace(
                    session,
                    next_turn_sequence=session.next_turn_sequence + 1,
                    updated_at=now,
                )
            await self._repository.add_turn(turn, message)
            events: list[UniversalEvent] = []
            session, queued_event = session.new_event(
                event_type="turn.queued",
                payload={
                    "type": "turn.queued",
                    "session_id": session_id,
                    "turn_id": turn_id,
                    "enqueue_sequence": turn.enqueue_sequence,
                },
                turn_id=turn_id,
                now=now,
            )
            await self._repository.add_event(session, queued_event)
            events.append(queued_event)
            if turn.status is TurnStatus.RUNNING:
                session, started_event = session.new_event(
                    event_type="turn.started",
                    payload={"type": "turn.started", "session_id": session_id, "turn_id": turn_id},
                    turn_id=turn_id,
                    now=now,
                )
                await self._repository.add_event(session, started_event)
                events.append(started_event)
            await self._repository.save_submission(session)
            return TurnSubmission(turn, tuple(events))

        return await self._tx.run_in_tx(operation)

    async def persist_runtime_event(
        self, session_id: str, turn_id: str, payload: dict[str, object]
    ) -> UniversalEvent:
        envelope = RuntimeEventEnvelope.parse(payload)
        if envelope.is_live or envelope.is_terminal:
            raise InvalidTurn("live and terminal events use dedicated workflow paths")

        async def operation() -> UniversalEvent:
            session = await self._repository.get(session_id, for_update=True)
            if session is None:
                raise RuntimeSessionNotFound(session_id)
            turn = await self._repository.get_turn(turn_id)
            if turn is None or turn.session_id != session_id:
                raise InvalidTurn(f"turn not found: {turn_id}")
            if envelope.is_tool_call and envelope.item_id:
                existing = await self._repository.find_completed_item(turn_id, envelope.item_id)
                if existing is not None:
                    return existing
            now = datetime.now(UTC)
            message: Message | None = None
            if envelope.creates_message:
                session, message = session.new_message(
                    turn_id=turn_id,
                    role=envelope.message_role,
                    content_ref=envelope.message_content,
                    now=now,
                )
            session, event = session.new_event(
                event_type=envelope.event_type,
                payload=payload,
                turn_id=turn_id,
                now=now,
            )
            await self._repository.add_event(session, event, message)
            return event

        return await self._tx.run_in_tx(operation)

    async def replay_events(
        self, session_id: str, after_sequence: int, *, limit: int = 200
    ) -> list[UniversalEvent]:
        if after_sequence < 0:
            raise InvalidTurn("after_sequence must not be negative")
        bounded = min(max(limit, 1), 1000)
        session = await self.get(session_id)
        return await self._tx.run_in_tx(
            lambda: self._repository.list_events_after(session.session_id, after_sequence, bounded)
        )

    async def list_messages(self, session_id: str, *, limit: int = 200) -> list[Message]:
        bounded = min(max(limit, 1), 1000)
        session = await self.get(session_id)
        return await self._tx.run_in_tx(
            lambda: self._repository.list_messages(session.session_id, bounded)
        )

    async def get_turn(self, turn_id: str) -> Turn:
        turn = await self._tx.run_in_tx(lambda: self._repository.get_turn(turn_id.strip()))
        if turn is None:
            raise InvalidTurn(f"turn not found: {turn_id}")
        return turn

    async def execution(self, session_id: str, turn_id: str) -> TurnExecution:
        async def operation() -> TurnExecution:
            session = await self._repository.get(session_id)
            turn = await self._repository.get_turn(turn_id)
            if session is None:
                raise RuntimeSessionNotFound(session_id)
            if turn is None or turn.session_id != session_id:
                raise InvalidTurn(f"turn not found: {turn_id}")
            if session.active_turn_id != turn_id or turn.status is not TurnStatus.RUNNING:
                raise InvalidTurn("turn is not the active running Turn")
            return TurnExecution(session, turn)

        return await self._tx.run_in_tx(operation)

    async def attach_runtime(self, session_id: str, turn_id: str, handle: str) -> RuntimeSession:
        async def operation() -> RuntimeSession:
            session = await self._repository.get(session_id, for_update=True)
            if session is None:
                raise RuntimeSessionNotFound(session_id)
            if session.active_turn_id != turn_id:
                raise InvalidTurn("turn is no longer active")
            attached = session.attach_runtime(handle, now=datetime.now(UTC))
            await self._repository.save_execution(attached)
            return attached

        return await self._tx.run_in_tx(operation)

    async def finalize_turn(
        self,
        session_id: str,
        turn_id: str,
        status: TurnStatus,
        *,
        error_class: str | None = None,
        terminal_payload: dict[str, object] | None = None,
    ) -> TurnFinalization:
        async def operation() -> TurnFinalization:
            session = await self._repository.get(session_id, for_update=True)
            turn = await self._repository.get_turn(turn_id, for_update=True)
            if session is None:
                raise RuntimeSessionNotFound(session_id)
            if turn is None or turn.session_id != session_id:
                raise InvalidTurn(f"turn not found: {turn_id}")
            now = datetime.now(UTC)
            terminal = turn.finish(status, now=now, error_class=error_class)
            released = session.release_turn(terminal, now=now)
            next_turn: Turn | None = None
            events: list[UniversalEvent] = []
            reason = {
                TurnStatus.COMPLETED: "completed",
                TurnStatus.FAILED: "error",
                TurnStatus.INTERRUPTED: "interrupted",
            }[status]
            if terminal_payload is not None and str(terminal_payload.get("reason") or "") == reason:
                payload: dict[str, object] = terminal_payload
            else:
                payload = {
                    "type": "session.ended",
                    "session_id": session.runtime_session_id or session_id,
                    "reason": reason,
                }
            released, terminal_event = released.new_event(
                event_type="session.ended", payload=payload, turn_id=turn_id, now=now
            )
            await self._repository.add_event(released, terminal_event)
            events.append(terminal_event)
            lifecycle_type = f"turn.{status.value}"
            released, lifecycle_event = released.new_event(
                event_type=lifecycle_type,
                payload={
                    "type": lifecycle_type,
                    "session_id": session_id,
                    "turn_id": turn_id,
                    "error_class": error_class,
                },
                turn_id=turn_id,
                now=now,
            )
            await self._repository.add_event(released, lifecycle_event)
            events.append(lifecycle_event)
            if status is TurnStatus.COMPLETED:
                head = await self._repository.get_queue_head(session_id)
                if head is not None:
                    released, next_turn = released.claim(head, now=now)
                    released, started_event = released.new_event(
                        event_type="turn.started",
                        payload={
                            "type": "turn.started",
                            "session_id": session_id,
                            "turn_id": next_turn.turn_id,
                        },
                        turn_id=next_turn.turn_id,
                        now=now,
                    )
                    await self._repository.add_event(released, started_event)
                    events.append(started_event)
            await self._repository.save_execution(released, terminal)
            if next_turn is not None:
                await self._repository.save_execution(released, next_turn)
            return TurnFinalization(terminal, next_turn, tuple(events))

        return await self._tx.run_in_tx(operation)


_SECRET_KEYS = {"api_key", "apikey", "authorization", "session_token", "access_token"}


def _contains_secret(value: object) -> bool:
    if isinstance(value, dict):
        return any(
            str(key).lower() in _SECRET_KEYS or _contains_secret(item)
            for key, item in value.items()
        )
    if isinstance(value, list):
        return any(_contains_secret(item) for item in value)
    return False
