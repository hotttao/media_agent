DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM media_workflow.templates WHERE organization_id<>'') OR EXISTS(SELECT 1 FROM media_workflow.workflows WHERE organization_id<>'' OR workspace_id<>'') THEN RAISE EXCEPTION 'cannot drop bound resource identity'; END IF;
END $$;
DROP TRIGGER template_resource_boundary ON media_workflow.templates;
DROP TRIGGER workflow_resource_boundary ON media_workflow.workflows;
DROP FUNCTION media_workflow.freeze_definition_boundary();
DROP INDEX media_workflow.templates_resource_org;
DROP INDEX media_workflow.workflows_resource_workspace;
DROP INDEX media_workflow.templates_scope_name_version_idx;
DROP INDEX media_workflow.workflows_scope_name_version_idx;
CREATE UNIQUE INDEX templates_scope_name_version_idx ON media_workflow.templates(scope,name,version) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX workflows_scope_name_version_idx ON media_workflow.workflows(scope,name,version) WHERE deleted_at IS NULL;
ALTER TABLE media_workflow.templates DROP COLUMN organization_id;
ALTER TABLE media_workflow.workflows DROP COLUMN organization_id,DROP COLUMN workspace_id;
ALTER TABLE media_workflow.template_versions DROP COLUMN status;
