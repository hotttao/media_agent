BEGIN;
ALTER TABLE media_workflow.step_runs
    DROP COLUMN IF EXISTS platform,
    DROP COLUMN IF EXISTS gen_way,
    DROP COLUMN IF EXISTS gen_type,
    DROP COLUMN IF EXISTS asset_kind,
    DROP COLUMN IF EXISTS needs_confirm,
    DROP COLUMN IF EXISTS candidate_count;
COMMIT;
