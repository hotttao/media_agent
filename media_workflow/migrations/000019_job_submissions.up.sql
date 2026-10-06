BEGIN;
CREATE TABLE media_workflow.job_submissions (
    submitted_by text NOT NULL CHECK (submitted_by <> ''),
    organization_id text NOT NULL CHECK (organization_id <> ''),
    workspace_id text NOT NULL CHECK (workspace_id <> ''),
    idempotency_key text NOT NULL CHECK (octet_length(idempotency_key) BETWEEN 1 AND 160 AND idempotency_key ~ '^[!-~]+$'),
    request_digest text NOT NULL CHECK (request_digest ~ '^[a-f0-9]{64}$'),
    job_id text NOT NULL UNIQUE REFERENCES media_workflow.jobs(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (submitted_by,organization_id,workspace_id,idempotency_key)
);
COMMIT;
