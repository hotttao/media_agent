BEGIN;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_workflow.execution_starts) THEN
        RAISE EXCEPTION 'execution start audit exists; refusing destructive downgrade';
    END IF;
END $$;
DROP TABLE media_workflow.execution_starts;
COMMIT;
