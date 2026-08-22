import httpx
import pytest
from fastapi import FastAPI

from adapter.biz.api.v1.deps import get_runtime_workspace
from adapter.biz.api.v1.routes.runtime_workspace.router import build_runtime_workspace_router
from adapter.infra.runtime_workspace import RuntimeWorkspaceIdentity


@pytest.mark.asyncio
async def test_runtime_workspace_route_returns_container_identity_only() -> None:
    app = FastAPI()
    identity = RuntimeWorkspaceIdentity(
        workspace_id="workspace-1",
        storage_id="storage-1",
        created_at="2026-08-22T00:00:00+00:00",
        mount_path="/workspace",
    )

    async def override() -> RuntimeWorkspaceIdentity:
        return identity

    app.dependency_overrides[get_runtime_workspace] = override
    app.include_router(build_runtime_workspace_router())
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://test"
    ) as client:
        response = await client.get("/api/v1/runtime-workspace")

    assert response.status_code == 200
    assert response.json() == {
        "workspace_id": "workspace-1",
        "storage_id": "storage-1",
        "created_at": "2026-08-22T00:00:00+00:00",
        "mount_path": "/workspace",
    }
