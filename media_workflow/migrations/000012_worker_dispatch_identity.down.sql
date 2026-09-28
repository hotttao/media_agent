BEGIN;

ALTER TABLE media_workflow.worker_dispatches
    DROP COLUMN IF EXISTS auth_source,
    DROP COLUMN IF EXISTS actor_id;

COMMIT;
