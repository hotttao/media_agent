BEGIN;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_workflow.job_submissions) THEN
        RAISE EXCEPTION 'cannot discard accepted submission audit';
    END IF;
END $$;
DROP TABLE media_workflow.job_submissions;
COMMIT;
