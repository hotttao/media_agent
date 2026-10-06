BEGIN;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_workflow.workflow_publications) OR
       EXISTS (SELECT 1 FROM media_workflow.workflows WHERE status IN ('publishing','publish_failed')) THEN
        RAISE EXCEPTION 'workflow publication facts prevent downgrade';
    END IF;
END; $$;
DROP TABLE media_workflow.workflow_publications;
DROP FUNCTION media_workflow.protect_workflow_publication();
ALTER TABLE media_workflow.workflows DROP CONSTRAINT workflows_status_check;
ALTER TABLE media_workflow.workflows ADD CONSTRAINT workflows_status_check CHECK (status IN ('draft','published','archived'));
COMMIT;
