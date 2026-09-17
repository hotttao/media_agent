BEGIN;
ALTER TABLE media_workflow.step_runs DROP CONSTRAINT IF EXISTS step_runs_approval_policy_check;
ALTER TABLE media_workflow.step_runs DROP COLUMN IF EXISTS approval_policy;
COMMIT;
