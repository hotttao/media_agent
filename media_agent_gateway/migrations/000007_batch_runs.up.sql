CREATE TABLE media_agent_gateway.batch_runs (
    id uuid PRIMARY KEY,
    workspace_id text NOT NULL,
    requested_by text NOT NULL,
    idempotency_key text NOT NULL,
    status text NOT NULL,
    item_count integer NOT NULL,
    next_cursor integer NOT NULL DEFAULT 0,
    completed_count integer NOT NULL DEFAULT 0,
    failed_count integer NOT NULL DEFAULT 0,
    cancelled_count integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL,
    finished_at timestamptz,
    updated_at timestamptz NOT NULL,
    CONSTRAINT batch_runs_status_check CHECK (status IN ('creating','running','completed','partial_failed','failed','cancelled')),
    CONSTRAINT batch_runs_counts_check CHECK (
        item_count > 0 AND next_cursor >= 0 AND next_cursor <= item_count AND
        completed_count >= 0 AND failed_count >= 0 AND cancelled_count >= 0 AND
        completed_count + failed_count + cancelled_count <= item_count
    ),
    UNIQUE (requested_by, workspace_id, idempotency_key)
);

CREATE TABLE media_agent_gateway.batch_run_items (
    batch_run_id uuid NOT NULL REFERENCES media_agent_gateway.batch_runs(id) ON DELETE CASCADE,
    item_key text NOT NULL,
    session_id uuid NOT NULL REFERENCES media_agent_gateway.agent_sessions(id),
    agent_task_id uuid REFERENCES media_agent_gateway.agent_tasks(id),
    input jsonb NOT NULL,
    status text NOT NULL,
    error_code text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (batch_run_id, item_key),
    UNIQUE (agent_task_id),
    CONSTRAINT batch_run_items_status_check CHECK (status IN ('pending','queued','running','completed','failed','cancelled'))
);

CREATE INDEX batch_runs_owner_workspace_updated_idx
    ON media_agent_gateway.batch_runs (requested_by, workspace_id, updated_at DESC);
CREATE INDEX batch_run_items_task_idx
    ON media_agent_gateway.batch_run_items (agent_task_id) WHERE agent_task_id IS NOT NULL;
