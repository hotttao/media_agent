from pydantic import BaseModel


class RuntimeWorkspaceResponse(BaseModel):
    workspace_id: str
    storage_id: str
    created_at: str
    mount_path: str
