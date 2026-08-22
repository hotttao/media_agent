CREATE TABLE IF NOT EXISTS share_metadata (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    owner_user_id text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL,
    revoked_at timestamptz,
    CONSTRAINT share_metadata_status_check CHECK (status IN ('active','revoked'))
);
CREATE INDEX IF NOT EXISTS idx_share_metadata_workspace_resource ON share_metadata(workspace_id,resource_type,resource_id,status);
