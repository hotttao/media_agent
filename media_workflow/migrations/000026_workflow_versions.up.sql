BEGIN;
CREATE TABLE media_workflow.workflow_versions (
 workflow_id TEXT NOT NULL REFERENCES media_workflow.workflows(id),
 version INTEGER NOT NULL CHECK(version>0),
 definition_snapshot JSONB NOT NULL CHECK(jsonb_typeof(definition_snapshot)='object'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(workflow_id,version)
);

-- Only surviving current rows are copied. Never infer a deleted old draft.
INSERT INTO media_workflow.workflow_versions(workflow_id,version,definition_snapshot,created_at)
SELECT w.id,w.version,jsonb_build_object(
 'scope',w.scope,'name',w.name,'description',COALESCE(w.description,''),'output_step_key',w.output_step_key,
 'steps',COALESCE((SELECT jsonb_agg(jsonb_build_object(
  'Key',s.step_key,'Name',COALESCE(s.step_name,''),'Order',s.step_order,'TemplateID',s.template_id,'TemplateVersion',s.template_version,
  'ApprovalPolicy',s.approval_policy,'ReusePolicy',s.reuse_policy,
  'SlotBindings',COALESCE((SELECT jsonb_agg(jsonb_build_object(
   'ToInput',b.to_input,'Source',b.source,'Ref',COALESCE(b.ref,''),'ConstValue',COALESCE(b.const_value,''),'DisplayOrder',b.display_order
  ) ORDER BY b.display_order,b.to_input) FROM media_workflow.slot_bindings b WHERE b.step_id=s.id),'[]'::jsonb)
 ) ORDER BY s.step_order,s.step_key) FROM media_workflow.workflow_steps s WHERE s.workflow_id=w.id AND s.version=w.version),'[]'::jsonb),
 'input_schema',COALESCE((SELECT jsonb_agg(CASE WHEN COALESCE(e->>'asset_kind','')='' THEN e-'asset_kind' ELSE e END ORDER BY e->>'name') FROM jsonb_array_elements(w.input_schema) e),'[]'::jsonb)
),w.updated_at FROM media_workflow.workflows w;

CREATE FUNCTION media_workflow.protect_workflow_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'workflow business version is immutable';
END $$;
CREATE TRIGGER frozen_workflow_version BEFORE UPDATE OR DELETE ON media_workflow.workflow_versions
FOR EACH ROW EXECUTE FUNCTION media_workflow.protect_workflow_version();
COMMIT;
