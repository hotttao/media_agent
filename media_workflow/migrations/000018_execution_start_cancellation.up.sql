BEGIN;
ALTER TABLE media_workflow.execution_starts DROP CONSTRAINT execution_starts_status_check;
DO $$ DECLARE instance_check text; BEGIN
    SELECT conname INTO STRICT instance_check FROM pg_constraint
    WHERE conrelid='media_workflow.execution_starts'::regclass AND contype='c'
      AND pg_get_constraintdef(oid) LIKE '%status%started%conductor_workflow_id%';
    EXECUTE format('ALTER TABLE media_workflow.execution_starts DROP CONSTRAINT %I',instance_check);
END $$;
ALTER TABLE media_workflow.execution_starts ADD CONSTRAINT execution_starts_status_check CHECK (status IN ('queued','uncertain','started','cancel_pending','cancelled'));
ALTER TABLE media_workflow.execution_starts ADD CONSTRAINT execution_starts_instance_state_check CHECK (
    (status IN ('started','cancel_pending') AND conductor_workflow_id IS NOT NULL)
    OR (status IN ('queued','uncertain') AND conductor_workflow_id IS NULL)
    OR status='cancelled'
);
DROP INDEX media_workflow.execution_starts_pending_idx;
CREATE INDEX execution_starts_pending_idx ON media_workflow.execution_starts(next_attempt_at,created_at,id) WHERE status IN ('queued','uncertain','cancel_pending');
COMMIT;
