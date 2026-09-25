BEGIN;

ALTER TABLE media_workflow.step_bindings
    DROP CONSTRAINT IF EXISTS step_bindings_source_value_check;

UPDATE media_workflow.step_bindings
SET ref = NULL
WHERE source = 'WORKFLOW_INPUT';

ALTER TABLE media_workflow.step_bindings
    ADD CONSTRAINT step_bindings_source_value_check CHECK (
        (source = 'WORKFLOW_INPUT' AND ref IS NULL AND const_value IS NULL) OR
        (source = 'STEP_OUTPUT' AND ref IS NOT NULL AND const_value IS NULL) OR
        (source = 'CONST' AND const_value IS NOT NULL AND ref IS NULL)
    );

COMMIT;
