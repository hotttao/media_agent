DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM media_workflow.workflow_versions) THEN
  RAISE EXCEPTION 'cannot drop workflow version history';
 END IF;
END $$;
DROP TRIGGER frozen_workflow_version ON media_workflow.workflow_versions;
DROP FUNCTION media_workflow.protect_workflow_version();
DROP TABLE media_workflow.workflow_versions;
