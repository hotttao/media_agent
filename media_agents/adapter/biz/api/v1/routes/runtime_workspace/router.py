"""Read-only diagnostics for the mounted Workspace Runtime Directory."""

from fastapi import APIRouter

from adapter.biz.api.v1.deps import RuntimeWorkspaceDep
from adapter.schemas.runtime_workspace.response import RuntimeWorkspaceResponse


def build_runtime_workspace_router() -> APIRouter:
    router = APIRouter(prefix="/api/v1/runtime-workspace", tags=["runtime-workspace"])

    @router.get("", response_model=RuntimeWorkspaceResponse)
    async def get(identity: RuntimeWorkspaceDep) -> RuntimeWorkspaceResponse:
        return RuntimeWorkspaceResponse(
            workspace_id=identity.workspace_id,
            storage_id=identity.storage_id,
            created_at=identity.created_at,
            mount_path=identity.mount_path,
        )

    return router
