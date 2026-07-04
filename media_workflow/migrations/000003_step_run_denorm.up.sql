-- step_runs 反范式化 template 字段（ClaimStepRun 免 join）
BEGIN;
ALTER TABLE media_workflow.step_runs ADD COLUMN IF NOT EXISTS platform text NOT NULL DEFAULT '';
ALTER TABLE media_workflow.step_runs ADD COLUMN IF NOT EXISTS gen_way text NOT NULL DEFAULT '';
ALTER TABLE media_workflow.step_runs ADD COLUMN IF NOT EXISTS gen_type text;
ALTER TABLE media_workflow.step_runs ADD COLUMN IF NOT EXISTS asset_kind text NOT NULL DEFAULT '';
ALTER TABLE media_workflow.step_runs ADD COLUMN IF NOT EXISTS needs_confirm boolean NOT NULL DEFAULT false;
ALTER TABLE media_workflow.step_runs ADD COLUMN IF NOT EXISTS candidate_count int NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS step_runs_claim_match_idx
    ON media_workflow.step_runs (status, platform, gen_way);
COMMIT;
