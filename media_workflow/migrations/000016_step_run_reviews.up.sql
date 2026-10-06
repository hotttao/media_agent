BEGIN;

CREATE TABLE media_workflow.step_run_reviews (
    id text PRIMARY KEY,
    job_id text NOT NULL REFERENCES media_workflow.jobs(id),
    step_run_id text NOT NULL REFERENCES media_workflow.step_runs(id),
    conductor_workflow_id text NOT NULL CHECK (conductor_workflow_id <> ''),
    conductor_task_id text NOT NULL CHECK (conductor_task_id <> ''),
    task_reference_name text NOT NULL CHECK (task_reference_name <> ''),
    generation integer NOT NULL CHECK (generation >= 0),
    decision text NOT NULL CHECK (decision IN ('accept', 'regenerate')),
    decided_by text NOT NULL CHECK (decided_by <> ''),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 160),
    selected_index integer,
    feedback text NOT NULL DEFAULT '' CHECK (length(feedback) <= 8192),
    candidates_snapshot jsonb NOT NULL CHECK (jsonb_typeof(candidates_snapshot) = 'array'),
    output_json jsonb NOT NULL CHECK (jsonb_typeof(output_json) = 'object'),
    request_digest text NOT NULL CHECK (request_digest ~ '^[a-f0-9]{64}$'),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','applied','failed')),
    claim_token text,
    claimed_at timestamptz,
    delivery_attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error text CHECK (last_error IS NULL OR last_error ~ '^[a-z0-9_]{0,64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    applied_at timestamptz,
    CHECK ((decision='accept' AND selected_index IS NOT NULL AND selected_index >= 0 AND feedback='') OR
           (decision='regenerate' AND selected_index IS NULL)),
    UNIQUE (step_run_id, conductor_task_id),
    UNIQUE (step_run_id, idempotency_key)
);
CREATE INDEX step_run_reviews_pending_idx
    ON media_workflow.step_run_reviews (next_attempt_at, created_at, id)
    WHERE status='pending';

COMMIT;
