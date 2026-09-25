BEGIN;

ALTER TABLE media_workflow.step_bindings RENAME TO slot_bindings;
ALTER INDEX IF EXISTS media_workflow.step_bindings_step_input_idx RENAME TO slot_bindings_step_input_idx;
ALTER TABLE media_workflow.slot_bindings
    RENAME CONSTRAINT step_bindings_source_value_check TO slot_bindings_source_value_check;

COMMIT;
