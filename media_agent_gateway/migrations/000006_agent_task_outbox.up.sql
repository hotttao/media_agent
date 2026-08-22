CREATE TABLE media_agent_gateway.agent_task_outbox (
    id uuid PRIMARY KEY,
    aggregate_id uuid NOT NULL REFERENCES media_agent_gateway.agent_tasks(id) ON DELETE CASCADE,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL,
    delivered_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT agent_task_outbox_attempts_check CHECK (attempts >= 0),
    UNIQUE (aggregate_id, event_type)
);

CREATE INDEX agent_task_outbox_pending_idx
    ON media_agent_gateway.agent_task_outbox (available_at, created_at)
    WHERE delivered_at IS NULL;
