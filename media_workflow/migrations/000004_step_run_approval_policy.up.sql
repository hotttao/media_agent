BEGIN;

ALTER TABLE media_workflow.step_runs
    ADD COLUMN IF NOT EXISTS approval_policy text NOT NULL DEFAULT 'when_multiple_candidates';

ALTER TABLE media_workflow.step_runs
    DROP CONSTRAINT IF EXISTS step_runs_approval_policy_check;

ALTER TABLE media_workflow.step_runs
    ADD CONSTRAINT step_runs_approval_policy_check
    CHECK (approval_policy IN ('when_multiple_candidates', 'always'));

COMMIT;
