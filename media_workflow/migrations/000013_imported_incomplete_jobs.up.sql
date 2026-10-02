BEGIN;

ALTER TABLE media_workflow.jobs
    DROP CONSTRAINT jobs_status_check;
ALTER TABLE media_workflow.jobs
    ADD CONSTRAINT jobs_status_check
    CHECK (status IN ('awaiting_resume','pending','running','need_confirm','succeeded','failed','cancelled'));

ALTER TABLE media_workflow.jobs
    ADD COLUMN origin text NOT NULL DEFAULT 'native',
    ADD COLUMN organization_id text,
    ADD COLUMN workspace_id text,
    ADD COLUMN source_system text,
    ADD COLUMN source_record_id text,
    ADD COLUMN source_digest text,
    ADD COLUMN imported_at timestamptz,
    ADD COLUMN resume_generation integer NOT NULL DEFAULT 0,
    ADD COLUMN resume_idempotency_key text,
    ADD CONSTRAINT jobs_origin_check CHECK (origin IN ('native','legacy_import')),
    ADD CONSTRAINT jobs_import_metadata_check CHECK (
        (origin = 'native' AND source_system IS NULL AND source_record_id IS NULL AND source_digest IS NULL AND imported_at IS NULL)
        OR (origin = 'legacy_import' AND organization_id IS NOT NULL AND organization_id <> ''
            AND workspace_id IS NOT NULL AND workspace_id <> ''
            AND source_system IS NOT NULL AND source_system <> ''
            AND source_record_id IS NOT NULL AND source_record_id <> ''
            AND source_digest ~ '^[0-9a-f]{64}$'
            AND imported_at IS NOT NULL)
    );
CREATE UNIQUE INDEX jobs_legacy_source_idx
    ON media_workflow.jobs (workspace_id, source_system, source_record_id)
    WHERE origin = 'legacy_import';

ALTER TABLE media_workflow.step_runs
    ADD COLUMN origin text NOT NULL DEFAULT 'native',
    ADD COLUMN source_system text,
    ADD COLUMN source_record_id text,
    ADD COLUMN source_sha256 text,
    ADD COLUMN source_created_at timestamptz,
    ADD CONSTRAINT step_runs_origin_check CHECK (origin IN ('native','legacy_import')),
    ADD CONSTRAINT step_runs_import_metadata_check CHECK (
        (origin = 'native' AND source_system IS NULL AND source_record_id IS NULL
            AND source_sha256 IS NULL AND source_created_at IS NULL)
        OR (origin = 'legacy_import' AND status = 'succeeded'
            AND source_system IS NOT NULL AND source_system <> ''
            AND source_record_id IS NOT NULL AND source_record_id <> ''
            AND source_sha256 ~ '^[0-9a-f]{64}$' AND source_created_at IS NOT NULL
            AND output_asset_id IS NOT NULL AND output_asset_id <> '')
    );

COMMIT;
