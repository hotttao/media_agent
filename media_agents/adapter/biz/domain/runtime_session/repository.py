"""Persistence port for the Runtime Session aggregate."""

from __future__ import annotations

from typing import Protocol

from adapter.biz.domain.runtime_session.entity import (
    AgentRequest,
    Message,
    RuntimeSession,
    Turn,
    UniversalEvent,
)


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

    async def add_event(
        self, session: RuntimeSession, event: UniversalEvent, message: Message | None = None
    ) -> None: ...

    async def find_completed_item(self, turn_id: str, item_id: str) -> UniversalEvent | None: ...

    async def list_events_after(
        self, session_id: str, after_sequence: int, limit: int
    ) -> list[UniversalEvent]: ...

    async def list_messages(self, session_id: str, limit: int) -> list[Message]: ...

    async def add_agent_request(
        self, session: RuntimeSession, request: AgentRequest, event: UniversalEvent
    ) -> None: ...

    async def get_agent_request(
        self, request_id: str, *, for_update: bool = False
    ) -> AgentRequest | None: ...

    async def save_agent_request(
        self, session: RuntimeSession, request: AgentRequest, event: UniversalEvent
    ) -> None: ...

    async def list_agent_requests(
        self, session_id: str, turn_id: str | None = None
    ) -> list[AgentRequest]: ...

    async def save_agent_request_state(self, request: AgentRequest) -> None: ...

    async def list_running_turns(self) -> list[Turn]: ...
