CREATE TABLE IF NOT EXISTS agent_templates (
    id text PRIMARY KEY,
    workspace_id text NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    owner_user_id text NOT NULL,
    name text NOT NULL,
    configuration jsonb NOT NULL,
    status text NOT NULL DEFAULT 'active',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT agent_templates_status_check CHECK (status IN ('active', 'archived'))
);

CREATE INDEX IF NOT EXISTS idx_agent_templates_owner_workspace_updated
    ON agent_templates (owner_user_id, workspace_id, updated_at DESC);
