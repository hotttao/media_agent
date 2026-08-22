CREATE TABLE IF NOT EXISTS public_exports (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    object_path text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    created_by text NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT public_exports_status_check CHECK (status IN ('pending','ready','failed','revoked'))
);
CREATE INDEX IF NOT EXISTS idx_public_exports_resource ON public_exports(workspace_id,resource_type,resource_id,created_at DESC);
