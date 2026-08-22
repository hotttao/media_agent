from datetime import UTC, datetime

import httpx
import pytest
from fastapi import FastAPI

from adapter.biz.api.v1.deps import get_execute_turn_flow, get_runtime_session_service
from adapter.biz.api.v1.routes.runtime_session.router import build_runtime_session_router
from adapter.biz.domain.runtime_session.entity import (
    AgentRequest,
    AgentRequestStatus,
    Message,
    RuntimeSession,
    Turn,
    TurnStatus,
    UniversalEvent,
)
from adapter.biz.domain.runtime_session.errors import (
    ContextVersionConflict,
    RuntimeSessionNotFound,
)


class FakeService:
    def __init__(self) -> None:
        self.value: RuntimeSession | None = None

    async def register(self, command: object) -> RuntimeSession:
        if self.value is not None:
            raise ContextVersionConflict("session-1", 1, 2)
        self.value = RuntimeSession.register(
            session_id="session-1",
            context_version=1,
            context_ref="context:v1",
            now=datetime(2026, 8, 22, tzinfo=UTC),
        )
        return self.value

    async def get(self, session_id: str) -> RuntimeSession:
        if self.value is None or session_id != self.value.session_id:
            raise RuntimeSessionNotFound(session_id)
        return self.value

    async def get_turn(self, turn_id: str) -> Turn:
        return _turn()

    async def replay_events(
        self, session_id: str, after_sequence: int, *, limit: int = 200
    ) -> list[UniversalEvent]:
        await self.get(session_id)
        event = UniversalEvent(
            "event-2",
            session_id,
            "turn-1",
            2,
            "item.completed",
            {"type": "item.completed"},
            datetime(2026, 8, 22, tzinfo=UTC),
        )
        return [event] if after_sequence < 2 and limit > 0 else []

    async def list_messages(self, session_id: str, *, limit: int = 200) -> list[Message]:
        await self.get(session_id)
        return [
            Message(
                "message-1",
                session_id,
                "turn-1",
                "assistant",
                {"text": "hello"},
                1,
                datetime(2026, 8, 22, tzinfo=UTC),
            )
        ][:limit]

    async def list_agent_requests(
        self, session_id: str, *, turn_id: str | None = None
    ) -> list[AgentRequest]:
        await self.get(session_id)
        return [_request()]


class FakeFlow:
    async def submit(self, command: object, credentials: object = None) -> Turn:
        return _turn()

    async def interrupt(self, session_id: str, turn_id: str) -> bool:
        return True

    async def resolve_request(
        self, session_id: str, turn_id: str, request_id: str, response: dict[str, object]
    ) -> AgentRequest:
        return _request().resolve(response, now=datetime(2026, 8, 22, tzinfo=UTC))


def _turn() -> Turn:
    return Turn(
        turn_id="turn-1",
        session_id="session-1",
        idempotency_key="request-1",
        enqueue_sequence=1,
        status=TurnStatus.RUNNING,
        input={"text": "hello"},
        context_version=1,
        created_at=datetime(2026, 8, 22, tzinfo=UTC),
        started_at=datetime(2026, 8, 22, tzinfo=UTC),
    )


def _request() -> AgentRequest:
    return AgentRequest(
        "agent-request-1",
        "session-1",
        "turn-1",
        "permission",
        {"options": [{"option_id": "allow-once"}]},
        AgentRequestStatus.PENDING,
        None,
        None,
        datetime(2026, 8, 22, tzinfo=UTC),
    )


@pytest.mark.asyncio
async def test_register_get_and_conflict_mapping() -> None:
    app = FastAPI()
    service = FakeService()

    async def override_service() -> FakeService:
        return service

    app.dependency_overrides[get_runtime_session_service] = override_service
    app.dependency_overrides[get_execute_turn_flow] = lambda: FakeFlow()
    app.include_router(build_runtime_session_router())
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://test"
    ) as client:
        created = await client.post(
            "/api/v1/runtime-sessions",
            json={
                "session_id": "session-1",
                "context_version": 1,
                "context_ref": "context:v1",
            },
        )
        assert created.status_code == 201
        assert created.json()["chat_status"] == "human"

        fetched = await client.get("/api/v1/runtime-sessions/session-1")
        assert fetched.status_code == 200
        assert fetched.json()["context_version"] == 1

        conflict = await client.post(
            "/api/v1/runtime-sessions",
            json={
                "session_id": "session-1",
                "context_version": 2,
                "context_ref": "context:v2",
            },
        )
        assert conflict.status_code == 409

        turn = await client.post(
            "/api/v1/runtime-sessions/session-1/turns",
            json={"idempotency_key": "request-1", "context_version": 1, "input": {"text": "hello"}},
        )
        assert turn.status_code == 202
        assert turn.json()["status"] == "running"

        fetched_turn = await client.get("/api/v1/runtime-sessions/session-1/turns/turn-1")
        assert fetched_turn.status_code == 200
        interrupted = await client.post("/api/v1/runtime-sessions/session-1/turns/turn-1/interrupt")
        assert interrupted.json() == {"interrupted": True}

        events = await client.get(
            "/api/v1/runtime-sessions/session-1/events?after_sequence=1&limit=10"
        )
        assert events.status_code == 200
        assert events.json()[0]["sequence"] == 2
        messages = await client.get("/api/v1/runtime-sessions/session-1/messages")
        assert messages.status_code == 200
        assert messages.json()[0]["role"] == "assistant"

        requests = await client.get("/api/v1/runtime-sessions/session-1/requests")
        assert requests.status_code == 200
        assert requests.json()[0]["status"] == "pending"
        resolved = await client.post(
            "/api/v1/runtime-sessions/session-1/turns/turn-1/requests/agent-request-1/resolve",
            json={"response": {"option_id": "allow-once"}},
        )
        assert resolved.status_code == 200
        assert resolved.json()["status"] == "resolved"


@pytest.mark.asyncio
async def test_get_missing_runtime_session_returns_not_found() -> None:
    app = FastAPI()
    service = FakeService()

    async def override_service() -> FakeService:
        return service

    app.dependency_overrides[get_runtime_session_service] = override_service
    app.dependency_overrides[get_execute_turn_flow] = lambda: FakeFlow()
    app.include_router(build_runtime_session_router())

    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://test"
    ) as client:
        response = await client.get("/api/v1/runtime-sessions/missing")

    assert response.status_code == 404
