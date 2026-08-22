CREATE TABLE IF NOT EXISTS runtime_environments (
    id text PRIMARY KEY,
    name text NOT NULL,
    endpoint text NOT NULL,
    capacity jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'active',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT runtime_environments_status_check CHECK (status IN ('active','draining','offline'))
);
CREATE TABLE IF NOT EXISTS workspace_placements (
    workspace_id text PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    environment_id text REFERENCES runtime_environments(id),
    status text NOT NULL,
    reason text,
    volume_ref text,
    version bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL,
    CONSTRAINT workspace_placements_status_check CHECK (status IN ('pending','placed','failed','unplaced'))
);
CREATE INDEX IF NOT EXISTS idx_runtime_environments_status ON runtime_environments(status, updated_at DESC);
