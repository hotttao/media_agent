CREATE TABLE media_agent_gateway.agent_tasks (
    id uuid PRIMARY KEY,
    workspace_id text NOT NULL,
    requested_by text NOT NULL,
    session_id uuid REFERENCES media_agent_gateway.agent_sessions(id) ON DELETE SET NULL,
    source text NOT NULL,
    source_resource_id text NOT NULL,
    idempotency_key text NOT NULL,
    input jsonb NOT NULL,
    status text NOT NULL DEFAULT 'queued',
    attempt integer NOT NULL DEFAULT 0,
    turn_id text,
    result_ref jsonb,
    error_code text,
    error_message text,
    created_at timestamptz NOT NULL,
    started_at timestamptz,
    finished_at timestamptz,
    updated_at timestamptz NOT NULL,
    CONSTRAINT agent_tasks_status_check CHECK (status IN ('queued','running','waiting_turn','retry_wait','completed','failed','cancelled')),
    CONSTRAINT agent_tasks_attempt_check CHECK (attempt >= 0),
    UNIQUE (requested_by, source, source_resource_id, idempotency_key)
);
CREATE INDEX agent_tasks_owner_workspace_updated_idx
    ON media_agent_gateway.agent_tasks (requested_by, workspace_id, updated_at DESC);
CREATE INDEX agent_tasks_dispatch_idx
    ON media_agent_gateway.agent_tasks (status, updated_at) WHERE status IN ('queued','retry_wait');
