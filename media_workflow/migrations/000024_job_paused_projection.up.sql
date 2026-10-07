BEGIN;
ALTER TABLE media_workflow.jobs DROP CONSTRAINT jobs_status_check;
ALTER TABLE media_workflow.jobs ADD CONSTRAINT jobs_status_check
    CHECK (status IN ('awaiting_resume','pending','running','paused','need_confirm','succeeded','failed','cancelled'));
COMMIT;
