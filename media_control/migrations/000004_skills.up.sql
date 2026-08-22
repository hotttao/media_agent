CREATE TABLE IF NOT EXISTS skills (
    id text PRIMARY KEY,
    workspace_id text NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    owner_user_id text NOT NULL,
    name text NOT NULL,
    content_path text NOT NULL,
    checksum text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT skills_status_check CHECK (status IN ('active', 'disabled', 'archived'))
);

CREATE INDEX IF NOT EXISTS idx_skills_owner_workspace_updated
    ON skills (owner_user_id, workspace_id, updated_at DESC);
