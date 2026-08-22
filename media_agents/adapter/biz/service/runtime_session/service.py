"""Single-domain Runtime Session use cases."""

from __future__ import annotations

from collections.abc import Awaitable, Callable
from typing import Protocol, TypeVar

from adapter.biz.domain.runtime_session.entity import RuntimeSession
from adapter.biz.domain.runtime_session.errors import (
    ContextVersionConflict,
    RuntimeSessionAlreadyExists,
    RuntimeSessionNotFound,
)
from adapter.biz.domain.runtime_session.repository import RuntimeSessionRepository
from adapter.biz.service.runtime_session.command import RegisterRuntimeSession

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
