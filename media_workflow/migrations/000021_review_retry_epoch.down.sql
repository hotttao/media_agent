BEGIN;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_workflow.step_run_reviews WHERE accepted_retry_count IS NOT NULL) THEN
        RAISE EXCEPTION 'approval retry epoch audit prevents downgrade';
    END IF;
END; $$;
DROP TRIGGER immutable_review_retry_epoch ON media_workflow.step_run_reviews;
DROP FUNCTION media_workflow.protect_review_retry_epoch();
ALTER TABLE media_workflow.step_run_reviews DROP COLUMN accepted_retry_count;
COMMIT;
