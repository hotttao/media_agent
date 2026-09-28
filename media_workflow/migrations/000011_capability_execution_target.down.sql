BEGIN;

ALTER TABLE media_workflow.execution_capabilities
    DROP CONSTRAINT IF EXISTS execution_capabilities_pool_check,
    DROP CONSTRAINT IF EXISTS execution_capabilities_target_check,
    DROP COLUMN IF EXISTS managed_pool_key,
    DROP COLUMN IF EXISTS execution_target;

COMMIT;
