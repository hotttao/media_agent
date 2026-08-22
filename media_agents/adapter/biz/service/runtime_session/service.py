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
)
from adapter.biz.domain.runtime_session.errors import (
    ContextVersionConflict,
    InvalidTurn,
    RuntimeSessionAlreadyExists,
    RuntimeSessionNotFound,
)
from adapter.biz.domain.runtime_session.repository import RuntimeSessionRepository
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
        session_id = command.session_id.strip()
        key = command.idempotency_key.strip()
        if not key:
            raise InvalidTurn("idempotency_key is required")
        if not command.input:
            raise InvalidTurn("input is required")
        if _contains_secret(command.input):
            raise InvalidTurn("input must not contain runtime credentials")

        async def operation() -> Turn:
            session = await self._repository.get(session_id, for_update=True)
            if session is None:
                raise RuntimeSessionNotFound(session_id)
            existing = await self._repository.get_turn_by_idempotency_key(session_id, key)
            if existing is not None:
                return existing
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
            message = Message(
                message_id=str(uuid4()),
                session_id=session_id,
                turn_id=turn_id,
                role="user",
                content_ref=command.input,
                sequence=session.next_turn_sequence,
                created_at=now,
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
            await self._repository.save_submission(session)
            return turn

        return await self._tx.run_in_tx(operation)

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
            if status is TurnStatus.COMPLETED:
                head = await self._repository.get_queue_head(session_id)
                if head is not None:
                    released, next_turn = released.claim(head, now=now)
            await self._repository.save_execution(released, terminal)
            if next_turn is not None:
                await self._repository.save_execution(released, next_turn)
            return TurnFinalization(terminal, next_turn)

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
