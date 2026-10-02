BEGIN;

CREATE TABLE media_workflow.resume_outbox (
    id text PRIMARY KEY,
    job_id text NOT NULL REFERENCES media_workflow.jobs(id),
    resume_generation integer NOT NULL CHECK (resume_generation > 0),
    idempotency_key text NOT NULL,
    correlation_id text NOT NULL,
    definition jsonb NOT NULL,
    task_domains jsonb NOT NULL,
    workflow_input jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','started')),
    conductor_workflow_id text,
    attempts integer NOT NULL DEFAULT 0,
    claimed_at timestamptz,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (job_id, resume_generation),
    UNIQUE (correlation_id)
);
CREATE INDEX resume_outbox_pending_idx
    ON media_workflow.resume_outbox (next_attempt_at, created_at, id)
    WHERE status = 'pending';

COMMIT;
