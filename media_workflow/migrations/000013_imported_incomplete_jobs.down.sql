BEGIN;

-- Refuse rollback while historical facts exist.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_workflow.jobs WHERE origin = 'legacy_import') THEN
        RAISE EXCEPTION 'cannot roll back imported Jobs';
    END IF;
END $$;

ALTER TABLE media_workflow.step_runs
    DROP CONSTRAINT step_runs_import_metadata_check,
    DROP CONSTRAINT step_runs_origin_check,
    DROP COLUMN source_created_at,
    DROP COLUMN source_sha256,
    DROP COLUMN source_record_id,
    DROP COLUMN source_digest,
    DROP COLUMN source_system,
    DROP COLUMN origin;

DROP INDEX media_workflow.jobs_legacy_source_idx;
ALTER TABLE media_workflow.jobs
    DROP CONSTRAINT jobs_import_metadata_check,
    DROP CONSTRAINT jobs_origin_check,
    DROP COLUMN resume_idempotency_key,
    DROP COLUMN resume_generation,
    DROP COLUMN imported_at,
    DROP COLUMN source_record_id,
    DROP COLUMN source_system,
    DROP COLUMN workspace_id,
    DROP COLUMN organization_id,
    DROP COLUMN origin;

ALTER TABLE media_workflow.jobs DROP CONSTRAINT jobs_status_check;
ALTER TABLE media_workflow.jobs ADD CONSTRAINT jobs_status_check
    CHECK (status IN ('pending','running','need_confirm','succeeded','failed','cancelled'));

COMMIT;
