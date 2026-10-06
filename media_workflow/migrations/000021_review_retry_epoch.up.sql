BEGIN;
ALTER TABLE media_workflow.step_run_reviews ADD COLUMN accepted_retry_count integer
    CHECK (accepted_retry_count IS NULL OR accepted_retry_count >= 0);
CREATE FUNCTION media_workflow.protect_review_retry_epoch()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.accepted_retry_count IS DISTINCT FROM OLD.accepted_retry_count THEN
        RAISE EXCEPTION 'approval retry epoch is immutable';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER immutable_review_retry_epoch BEFORE UPDATE
ON media_workflow.step_run_reviews FOR EACH ROW
EXECUTE FUNCTION media_workflow.protect_review_retry_epoch();
COMMIT;
