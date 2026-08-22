CREATE TABLE IF NOT EXISTS tags (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name text NOT NULL,
    color text,
    created_by text NOT NULL,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE(workspace_id, name)
);
CREATE TABLE IF NOT EXISTS resource_tags (
    tag_id text NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY(tag_id, resource_type, resource_id)
);
CREATE INDEX IF NOT EXISTS idx_resource_tags_resource ON resource_tags(workspace_id, resource_type, resource_id);
