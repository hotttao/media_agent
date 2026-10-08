ALTER TABLE media_workflow.templates ADD COLUMN organization_id text NOT NULL DEFAULT '';
ALTER TABLE media_workflow.workflows ADD COLUMN organization_id text NOT NULL DEFAULT '', ADD COLUMN workspace_id text NOT NULL DEFAULT '';
ALTER TABLE media_workflow.template_versions ADD COLUMN status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published'));
-- Only the current explicitly published version has a provable publication fact.
UPDATE media_workflow.template_versions v SET status='published' FROM media_workflow.templates t WHERE v.template_id=t.id AND v.version=t.version AND t.status='published';
CREATE INDEX templates_resource_org ON media_workflow.templates(organization_id,created_by) WHERE deleted_at IS NULL;
CREATE INDEX workflows_resource_workspace ON media_workflow.workflows(organization_id,workspace_id) WHERE deleted_at IS NULL;
DROP INDEX media_workflow.templates_scope_name_version_idx;
CREATE UNIQUE INDEX templates_scope_name_version_idx ON media_workflow.templates(organization_id,scope,name,version) WHERE deleted_at IS NULL;
DROP INDEX media_workflow.workflows_scope_name_version_idx;
CREATE UNIQUE INDEX workflows_scope_name_version_idx ON media_workflow.workflows(organization_id,workspace_id,scope,name,version) WHERE deleted_at IS NULL;
CREATE FUNCTION media_workflow.freeze_definition_boundary() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.organization_id<>'' AND NEW.organization_id IS DISTINCT FROM OLD.organization_id THEN RAISE EXCEPTION 'resource organization is immutable'; END IF;
 IF TG_TABLE_NAME='workflows' AND to_jsonb(OLD)->>'workspace_id'<>'' AND to_jsonb(NEW)->>'workspace_id' IS DISTINCT FROM to_jsonb(OLD)->>'workspace_id' THEN RAISE EXCEPTION 'resource workspace is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER template_resource_boundary BEFORE UPDATE ON media_workflow.templates FOR EACH ROW EXECUTE FUNCTION media_workflow.freeze_definition_boundary();
CREATE TRIGGER workflow_resource_boundary BEFORE UPDATE ON media_workflow.workflows FOR EACH ROW EXECUTE FUNCTION media_workflow.freeze_definition_boundary();
