ALTER TABLE media_workflow.workflow_steps
  ADD COLUMN approval_policy TEXT NOT NULL DEFAULT 'when_multiple_candidates',
  ADD COLUMN reuse_policy TEXT NOT NULL DEFAULT 'reuse_if_available';

ALTER TABLE media_workflow.workflow_steps
  ADD CONSTRAINT workflow_steps_approval_policy_check
    CHECK (approval_policy IN ('when_multiple_candidates', 'always')),
  ADD CONSTRAINT workflow_steps_reuse_policy_check
    CHECK (reuse_policy IN ('reuse_if_available', 'force_new'));
