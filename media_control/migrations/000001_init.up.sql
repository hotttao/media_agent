CREATE TABLE IF NOT EXISTS workspaces (
    id text PRIMARY KEY,
    owner_user_id text NOT NULL,
    name text NOT NULL,
    desired_state text NOT NULL DEFAULT 'running',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_workspaces_owner_updated
    ON workspaces (owner_user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS agent_configurations (
    id text PRIMARY KEY,
    workspace_id text NOT NULL UNIQUE REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id text NOT NULL,
    agent_type text NOT NULL,
    encrypted_api_key bytea,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (workspace_id, agent_id)
);
