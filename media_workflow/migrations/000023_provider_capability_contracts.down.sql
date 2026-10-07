BEGIN;

DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_workflow.execution_capabilities WHERE output_media_type IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot downgrade while provider capability contracts exist';
    END IF;
END $$;

DROP TRIGGER protect_provider_capability_contract ON media_workflow.execution_capabilities;
DROP FUNCTION media_workflow.protect_provider_capability_contract();
ALTER TABLE media_workflow.execution_capabilities DROP CONSTRAINT execution_capabilities_output_contract_check;
DROP INDEX media_workflow.execution_capabilities_provider_identity;
ALTER TABLE media_workflow.execution_capabilities DROP COLUMN output_media_type;
ALTER TABLE media_workflow.execution_capabilities DROP COLUMN generation_method;
ALTER TABLE media_workflow.execution_capabilities DROP COLUMN generation_type;

COMMIT;
