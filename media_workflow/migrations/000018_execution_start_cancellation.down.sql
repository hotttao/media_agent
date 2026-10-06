BEGIN;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_workflow.execution_starts WHERE status IN ('cancel_pending','cancelled')) THEN
        RAISE EXCEPTION 'cannot discard execution start cancellation audit';
    END IF;
END $$;
ALTER TABLE media_workflow.execution_starts DROP CONSTRAINT execution_starts_instance_state_check;
ALTER TABLE media_workflow.execution_starts DROP CONSTRAINT execution_starts_status_check;
ALTER TABLE media_workflow.execution_starts ADD CONSTRAINT execution_starts_status_check CHECK (status IN ('queued','uncertain','started'));
ALTER TABLE media_workflow.execution_starts ADD CONSTRAINT execution_starts_check2 CHECK ((status='started')=(conductor_workflow_id IS NOT NULL));
DROP INDEX media_workflow.execution_starts_pending_idx;
CREATE INDEX execution_starts_pending_idx ON media_workflow.execution_starts(next_attempt_at,created_at,id) WHERE status IN ('queued','uncertain');
COMMIT;
