BEGIN;
-- Same immutable-provider rule as migration 000023: do not delete or silently
-- rewrite a published capability version. Retire it through a forward migration.
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM media_workflow.execution_capabilities
        WHERE capability_key IN ('gpt.t2i.text_prompt','gpt.i2i.multi_image_prompt') AND version=1
    ) THEN
        RAISE EXCEPTION 'cannot downgrade published GPT capability contracts; retire them with a forward migration';
    END IF;
END $$;
COMMIT;
