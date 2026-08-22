"""Single-domain Runtime Session use cases."""

from __future__ import annotations

from collections.abc import Awaitable, Callable
from dataclasses import replace
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
