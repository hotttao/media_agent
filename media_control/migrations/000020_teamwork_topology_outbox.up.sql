CREATE TABLE IF NOT EXISTS teamwork_topology_outbox (
    id uuid PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    topology_id text NOT NULL REFERENCES teamwork_topologies(id) ON DELETE CASCADE,
    topology_version bigint NOT NULL,
    status text NOT NULL,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL,
    delivered_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT teamwork_topology_outbox_version_check CHECK (topology_version > 0),
    CONSTRAINT teamwork_topology_outbox_status_check CHECK (status IN ('active','archived','deleted')),
    CONSTRAINT teamwork_topology_outbox_type_check CHECK (event_type = 'teamwork_topology.changed'),
    CONSTRAINT teamwork_topology_outbox_attempts_check CHECK (attempts >= 0),
    UNIQUE (topology_id, topology_version)
);

CREATE INDEX IF NOT EXISTS teamwork_topology_outbox_pending_idx
    ON teamwork_topology_outbox (available_at, created_at)
    WHERE delivered_at IS NULL;
