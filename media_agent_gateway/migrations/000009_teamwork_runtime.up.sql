CREATE TABLE media_agent_gateway.teamwork_topology_versions (
    topology_id text PRIMARY KEY,
    workspace_id text NOT NULL,
    version bigint NOT NULL,
    status text NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT teamwork_topology_versions_version_check CHECK (version > 0),
    CONSTRAINT teamwork_topology_versions_status_check CHECK (status IN ('active','archived','deleted'))
);

CREATE TABLE media_agent_gateway.teamwork_topology_inbox (
    event_id uuid PRIMARY KEY,
    workspace_id text NOT NULL,
    topology_id text NOT NULL,
    topology_version bigint NOT NULL,
    status text NOT NULL,
    processed_at timestamptz NOT NULL,
    CONSTRAINT teamwork_topology_inbox_version_check CHECK (topology_version > 0),
    CONSTRAINT teamwork_topology_inbox_status_check CHECK (status IN ('active','archived','deleted'))
);

ALTER TABLE media_agent_gateway.agent_tasks
    ADD COLUMN topology_id text,
    ADD COLUMN topology_version bigint,
    ADD CONSTRAINT agent_tasks_topology_reference_check CHECK (
        (topology_id IS NULL AND topology_version IS NULL) OR
        (topology_id IS NOT NULL AND topology_version > 0)
    );

CREATE INDEX agent_tasks_topology_status_idx
    ON media_agent_gateway.agent_tasks (topology_id, topology_version, status)
    WHERE topology_id IS NOT NULL;
