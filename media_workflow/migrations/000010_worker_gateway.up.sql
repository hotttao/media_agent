BEGIN;

CREATE TABLE IF NOT EXISTS media_workflow.worker_dispatches (
    id                    text PRIMARY KEY,
    conductor_task_id     text NOT NULL,
    conductor_workflow_id text NOT NULL,
    job_id                text NOT NULL REFERENCES media_workflow.jobs(id) ON DELETE CASCADE,
    step_run_id           text NOT NULL REFERENCES media_workflow.step_runs(id) ON DELETE CASCADE,
    attempt               integer NOT NULL CHECK (attempt > 0),
    capability_key        text NOT NULL,
    capability_version    integer NOT NULL,
    user_id               text NOT NULL,
    organization_id       text NOT NULL,
    credential_id         text NOT NULL,
    worker_instance_id    text NOT NULL,
    status                text NOT NULL CHECK (status IN ('claimed','delivered','running','result_received','expired','failed')),
    input_json            jsonb NOT NULL DEFAULT '{}'::jsonb,
    lease_expires_at      timestamptz NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    delivered_at          timestamptz,
    completed_at          timestamptz,
    UNIQUE (conductor_task_id, attempt)
);

CREATE INDEX IF NOT EXISTS worker_dispatches_active_worker_idx
    ON media_workflow.worker_dispatches (user_id, organization_id, credential_id, worker_instance_id, capability_key, capability_version, lease_expires_at)
    WHERE status IN ('claimed','delivered','running');

CREATE TABLE IF NOT EXISTS media_workflow.worker_result_inbox (
    id               text PRIMARY KEY,
    dispatch_id      text NOT NULL REFERENCES media_workflow.worker_dispatches(id) ON DELETE CASCADE,
    attempt          integer NOT NULL,
    operation        text NOT NULL CHECK (operation IN ('complete','fail')),
    idempotency_key  text NOT NULL,
    result_json      jsonb NOT NULL DEFAULT '{}'::jsonb,
    result_digest    text NOT NULL,
    conductor_status text NOT NULL DEFAULT 'pending' CHECK (conductor_status IN ('pending','submitted','failed')),
    last_error       text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    submitted_at     timestamptz,
    UNIQUE (dispatch_id, attempt, operation),
    UNIQUE (dispatch_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS worker_result_inbox_pending_idx
    ON media_workflow.worker_result_inbox (created_at)
    WHERE conductor_status IN ('pending','failed');

COMMIT;
