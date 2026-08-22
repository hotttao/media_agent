CREATE TABLE IF NOT EXISTS teamwork_topologies (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name text NOT NULL,
    entry_member_id text NOT NULL,
    version bigint NOT NULL DEFAULT 1,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT teamwork_topologies_status_check CHECK (status IN ('active','archived','deleted'))
);
CREATE TABLE IF NOT EXISTS teamwork_members (
    topology_id text NOT NULL REFERENCES teamwork_topologies(id) ON DELETE CASCADE,
    member_id text NOT NULL,
    agent_config_id text NOT NULL,
    name text NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (topology_id, member_id)
);
CREATE TABLE IF NOT EXISTS teamwork_edges (
    topology_id text NOT NULL REFERENCES teamwork_topologies(id) ON DELETE CASCADE,
    from_member_id text NOT NULL,
    to_member_id text NOT NULL,
    mode text NOT NULL,
    configuration jsonb NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (topology_id, from_member_id, to_member_id),
    FOREIGN KEY (topology_id, from_member_id) REFERENCES teamwork_members(topology_id, member_id) ON DELETE CASCADE,
    FOREIGN KEY (topology_id, to_member_id) REFERENCES teamwork_members(topology_id, member_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_teamwork_topologies_workspace ON teamwork_topologies(workspace_id, updated_at DESC);
