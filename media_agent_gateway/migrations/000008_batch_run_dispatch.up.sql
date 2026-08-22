CREATE TABLE media_agent_gateway.batch_run_outbox (
    id uuid PRIMARY KEY,
    batch_run_id uuid NOT NULL REFERENCES media_agent_gateway.batch_runs(id) ON DELETE CASCADE,
    action text NOT NULL,
    payload jsonb NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL,
    delivered_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT batch_run_outbox_action_check CHECK (action IN ('batch_run.dispatch','batch_run.cancel')),
    UNIQUE (batch_run_id, action)
);
CREATE INDEX batch_run_outbox_pending_idx ON media_agent_gateway.batch_run_outbox (available_at, created_at) WHERE delivered_at IS NULL;

CREATE TABLE media_agent_gateway.batch_run_inbox (
    event_id uuid PRIMARY KEY,
    batch_run_id uuid NOT NULL REFERENCES media_agent_gateway.batch_runs(id) ON DELETE CASCADE,
    item_key text NOT NULL,
    created_at timestamptz NOT NULL,
    FOREIGN KEY (batch_run_id, item_key) REFERENCES media_agent_gateway.batch_run_items(batch_run_id, item_key) ON DELETE CASCADE
);

INSERT INTO media_agent_gateway.batch_run_outbox (id, batch_run_id, action, payload, available_at, created_at, updated_at)
SELECT gen_random_uuid(), id, 'batch_run.dispatch', jsonb_build_object('batch_run_id', id), now(), now(), now()
FROM media_agent_gateway.batch_runs
WHERE status IN ('creating','running')
ON CONFLICT (batch_run_id, action) DO NOTHING;
