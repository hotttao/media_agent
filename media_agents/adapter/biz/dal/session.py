"""Application transaction boundary with context-local SQLAlchemy sessions."""

from __future__ import annotations

from collections.abc import Awaitable, Callable
from contextvars import ContextVar
from typing import TypeVar

from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

T = TypeVar("T")
_session_context: ContextVar[AsyncSession | None] = ContextVar("dal_session", default=None)


class TxRunner:
    def __init__(self, session_factory: async_sessionmaker[AsyncSession]) -> None:
        self._session_factory = session_factory

    async def run_in_tx(self, operation: Callable[[], Awaitable[T]]) -> T:
        async with self._session_factory() as session:
            token = _session_context.set(session)
            try:
                result = await operation()
                await session.commit()
                return result
            except Exception:
                await session.rollback()
                raise
            finally:
                _session_context.reset(token)


def current_session() -> AsyncSession:
    session = _session_context.get()
    if session is None:
        raise RuntimeError("repository operation requires an active transaction")
    return session
