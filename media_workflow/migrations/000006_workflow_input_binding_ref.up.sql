BEGIN;

DO $$
DECLARE constraint_name text;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid = 'media_workflow.step_bindings'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) LIKE '%WORKFLOW_INPUT%'
    LIMIT 1;
    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE media_workflow.step_bindings DROP CONSTRAINT %I', constraint_name);
    END IF;
END $$;

UPDATE media_workflow.step_bindings
SET ref = to_input
WHERE source = 'WORKFLOW_INPUT' AND ref IS NULL;

ALTER TABLE media_workflow.step_bindings
    ADD CONSTRAINT step_bindings_source_value_check CHECK (
        (source = 'WORKFLOW_INPUT' AND ref IS NOT NULL AND const_value IS NULL) OR
        (source = 'STEP_OUTPUT' AND ref IS NOT NULL AND const_value IS NULL) OR
        (source = 'CONST' AND const_value IS NOT NULL AND ref IS NULL)
    );

COMMIT;
