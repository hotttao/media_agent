CREATE TABLE IF NOT EXISTS memory_stores (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    owner_user_id text NOT NULL,
    name text NOT NULL,
    store_type text NOT NULL,
    configuration jsonb NOT NULL,
    credential_reference_id text NULL REFERENCES credential_references(id),
    status text NOT NULL DEFAULT 'active',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT memory_stores_status_check CHECK (status IN ('active', 'disabled'))
);

CREATE INDEX IF NOT EXISTS idx_memory_stores_workspace_updated
    ON memory_stores (workspace_id, updated_at DESC);
