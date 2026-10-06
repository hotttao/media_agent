BEGIN;
DROP TRIGGER frozen_workflow_compilation ON media_workflow.workflow_compilations;
DROP FUNCTION media_workflow.protect_frozen_workflow_compilation();
COMMIT;
