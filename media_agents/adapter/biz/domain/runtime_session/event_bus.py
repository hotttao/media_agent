"""Session Event Bus port; durable sequence remains owned by PostgreSQL."""

from __future__ import annotations

from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Protocol


@dataclass(frozen=True, slots=True)
class SessionEventNotification:
    session_id: str
    sequence: int
    payload: dict[str, object]


class SessionEventBus(Protocol):
    async def publish(self, notification: SessionEventNotification) -> None: ...

    def subscribe(self, session_id: str) -> AsyncIterator[SessionEventNotification]: ...

    async def close(self) -> None: ...
