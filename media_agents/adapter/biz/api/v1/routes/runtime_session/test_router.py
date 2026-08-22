from datetime import UTC, datetime

import httpx
import pytest
from fastapi import FastAPI

from adapter.biz.api.v1.deps import get_runtime_session_service
from adapter.biz.api.v1.routes.runtime_session.router import build_runtime_session_router
from adapter.biz.domain.runtime_session.entity import RuntimeSession
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


@pytest.mark.asyncio
async def test_register_get_and_conflict_mapping() -> None:
    app = FastAPI()
    service = FakeService()

    async def override_service() -> FakeService:
        return service

    app.dependency_overrides[get_runtime_session_service] = override_service
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


@pytest.mark.asyncio
async def test_get_missing_runtime_session_returns_not_found() -> None:
    app = FastAPI()
    service = FakeService()

    async def override_service() -> FakeService:
        return service

    app.dependency_overrides[get_runtime_session_service] = override_service
    app.include_router(build_runtime_session_router())

    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://test"
    ) as client:
        response = await client.get("/api/v1/runtime-sessions/missing")

    assert response.status_code == 404
