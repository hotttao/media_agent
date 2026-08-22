"""Persistence port for the Runtime Session aggregate."""

from __future__ import annotations

from typing import Protocol

from adapter.biz.domain.runtime_session.entity import Message, RuntimeSession, Turn


class RuntimeSessionRepository(Protocol):
    async def get(self, session_id: str, *, for_update: bool = False) -> RuntimeSession | None: ...

    async def add(self, session: RuntimeSession) -> None: ...

    async def get_turn_by_idempotency_key(
        self, session_id: str, idempotency_key: str
    ) -> Turn | None: ...

    async def add_turn(self, turn: Turn, message: Message) -> None: ...

    async def save_submission(self, session: RuntimeSession) -> None: ...

    async def get_turn(self, turn_id: str, *, for_update: bool = False) -> Turn | None: ...

    async def get_queue_head(self, session_id: str) -> Turn | None: ...

    async def save_execution(self, session: RuntimeSession, turn: Turn | None = None) -> None: ...
