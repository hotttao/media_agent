BEGIN;

ALTER TABLE media_workflow.slot_bindings
    RENAME CONSTRAINT slot_bindings_source_value_check TO step_bindings_source_value_check;
ALTER INDEX IF EXISTS media_workflow.slot_bindings_step_input_idx RENAME TO step_bindings_step_input_idx;
ALTER TABLE media_workflow.slot_bindings RENAME TO step_bindings;

COMMIT;
