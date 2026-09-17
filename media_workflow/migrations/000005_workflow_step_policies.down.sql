ALTER TABLE media_workflow.workflow_steps
  DROP CONSTRAINT IF EXISTS workflow_steps_approval_policy_check,
  DROP CONSTRAINT IF EXISTS workflow_steps_reuse_policy_check,
  DROP COLUMN IF EXISTS approval_policy,
  DROP COLUMN IF EXISTS reuse_policy;
