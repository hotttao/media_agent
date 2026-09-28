BEGIN;

ALTER TABLE media_workflow.execution_capabilities
    ADD COLUMN execution_target text NOT NULL DEFAULT 'personal_worker',
    ADD COLUMN managed_pool_key text;

ALTER TABLE media_workflow.execution_capabilities
    ADD CONSTRAINT execution_capabilities_target_check
        CHECK (execution_target IN ('personal_worker', 'managed_worker')),
    ADD CONSTRAINT execution_capabilities_pool_check
        CHECK (
            (execution_target = 'personal_worker' AND managed_pool_key IS NULL)
            OR
            (execution_target = 'managed_worker' AND managed_pool_key IS NOT NULL AND btrim(managed_pool_key) <> '')
        );

COMMIT;
