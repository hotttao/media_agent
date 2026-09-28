BEGIN;

ALTER TABLE media_workflow.worker_dispatches
    ADD COLUMN actor_id text NOT NULL DEFAULT 'Worker:legacy',
    ADD COLUMN auth_source text NOT NULL DEFAULT 'legacy_service_token';

ALTER TABLE media_workflow.worker_dispatches
    ALTER COLUMN actor_id DROP DEFAULT,
    ALTER COLUMN auth_source DROP DEFAULT;

COMMIT;
