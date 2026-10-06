BEGIN;

DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_workflow.step_run_reviews) THEN
        RAISE EXCEPTION 'review audit rows must be preserved before rollback';
    END IF;
END $$;
DROP TABLE media_workflow.step_run_reviews;

COMMIT;
