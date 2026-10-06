BEGIN;
CREATE TABLE media_workflow.execution_starts (
    id text PRIMARY KEY CHECK (id <> ''),
    job_id text NOT NULL UNIQUE REFERENCES media_workflow.jobs(id) ON DELETE RESTRICT,
    workflow_name text NOT NULL CHECK (workflow_name <> ''),
    workflow_version integer NOT NULL CHECK (workflow_version > 0),
    compilation_hash text NOT NULL CHECK (compilation_hash ~ '^[a-f0-9]{64}$'),
    correlation_id text NOT NULL UNIQUE CHECK (correlation_id = job_id),
    workflow_input jsonb NOT NULL CHECK (jsonb_typeof(workflow_input)='object' AND workflow_input ? 'job_id' AND jsonb_typeof(workflow_input->'job_id')='string' AND workflow_input->>'job_id'=job_id),
    task_domains jsonb NOT NULL CHECK (jsonb_typeof(task_domains)='object'),
    payload_digest text NOT NULL CHECK (payload_digest ~ '^[a-f0-9]{64}$'),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','uncertain','started')),
    claim_token text,
    claimed_at timestamptz,
    delivery_attempts integer NOT NULL DEFAULT 0 CHECK (delivery_attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    conductor_workflow_id text,
    last_error text NOT NULL DEFAULT '' CHECK (last_error ~ '^[a-z0-9_]{0,64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((claim_token IS NULL)=(claimed_at IS NULL)),
    CHECK (claim_token IS NULL OR claim_token <> ''),
    CHECK ((status='started')=(conductor_workflow_id IS NOT NULL)),
    CHECK (conductor_workflow_id IS NULL OR conductor_workflow_id <> '')
);
CREATE INDEX execution_starts_pending_idx ON media_workflow.execution_starts(next_attempt_at,created_at,id) WHERE status IN ('queued','uncertain');
COMMIT;
