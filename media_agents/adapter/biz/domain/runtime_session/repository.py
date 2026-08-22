"""Persistence port for the Runtime Session aggregate."""

from __future__ import annotations

from typing import Protocol

from adapter.biz.domain.runtime_session.entity import RuntimeSession


class RuntimeSessionRepository(Protocol):
    async def get(self, session_id: str, *, for_update: bool = False) -> RuntimeSession | None: ...

    async def add(self, session: RuntimeSession) -> None: ...
