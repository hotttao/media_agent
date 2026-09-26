BEGIN;

ALTER TABLE media_workflow.jobs
    DROP COLUMN IF EXISTS compilation_hash,
    DROP COLUMN IF EXISTS conductor_workflow_id;

DROP TABLE IF EXISTS media_workflow.workflow_compilations;
DROP TABLE IF EXISTS media_workflow.execution_capabilities;

ALTER TABLE media_workflow.template_inputs
    DROP COLUMN IF EXISTS asset_kind;

ALTER TABLE media_workflow.template_versions
    DROP COLUMN IF EXISTS invocation_mapping,
    DROP COLUMN IF EXISTS capability_version,
    DROP COLUMN IF EXISTS capability_key;

ALTER TABLE media_workflow.templates
    DROP COLUMN IF EXISTS invocation_mapping,
    DROP COLUMN IF EXISTS capability_version,
    DROP COLUMN IF EXISTS capability_key;

COMMIT;
