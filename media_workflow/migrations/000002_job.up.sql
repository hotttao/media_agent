-- =====================================================================
-- media_workflow — job + step_runs（P3）
-- =====================================================================
-- jobs: workflow 执行实例（含 JobInput bundle 快照）
-- step_runs: 单步执行（pull 模式 + NEED_CONFIRM）
-- id: text（uuid/nanoid）
-- =====================================================================

BEGIN;

CREATE TABLE IF NOT EXISTS media_workflow.jobs (
    id              text PRIMARY KEY,
    scope           text NOT NULL,
    workflow_id     text NOT NULL,
    workflow_version int NOT NULL,
    input           jsonb NOT NULL,                         -- JobInput bundle 快照
    status          text NOT NULL DEFAULT 'pending'         -- pending/running/need_confirm/succeeded/failed/cancelled
                    CHECK (status IN ('pending','running','need_confirm','succeeded','failed','cancelled')),
    retry_count     int NOT NULL DEFAULT 0,
    retry_history   jsonb NOT NULL DEFAULT '[]',            -- [{retry_at, reason, input_override?}]
    submitted_by    text NOT NULL,
    submitted_at    timestamptz NOT NULL DEFAULT now(),
    started_at      timestamptz,
    finished_at     timestamptz,
    failure_reason  text
);
CREATE INDEX IF NOT EXISTS jobs_scope_status_idx
    ON media_workflow.jobs (scope, status, submitted_at DESC);
CREATE INDEX IF NOT EXISTS jobs_workflow_idx
    ON media_workflow.jobs (workflow_id, workflow_version);
CREATE INDEX IF NOT EXISTS jobs_submitted_by_idx
    ON media_workflow.jobs (submitted_by, submitted_at DESC);

CREATE TABLE IF NOT EXISTS media_workflow.step_runs (
    id              text PRIMARY KEY,
    job_id          text NOT NULL REFERENCES media_workflow.jobs(id) ON DELETE CASCADE,
    step_key        text NOT NULL,
    step_order      int NOT NULL,
    template_id     text NOT NULL,
    template_version int NOT NULL,
    status          text NOT NULL DEFAULT 'blocked'        -- blocked/pending/claimed/need_confirm/succeeded/failed/skipped
                    CHECK (status IN ('blocked','pending','claimed','need_confirm','succeeded','failed','skipped')),
    retry_count     int NOT NULL DEFAULT 0,
    resolved_inputs jsonb NOT NULL DEFAULT '{}',           -- {slot_name: {resource_type,id,url,content}}
    rendered_prompt text,
    candidates      jsonb NOT NULL DEFAULT '[]',            -- [{index,url,content,is_primary}]
    selected_index  int,
    output_asset_id text,                                   -- → media_materials.generated_assets.id
    failure_reason  text,
    claimed_by      text,
    claimed_at      timestamptz,
    started_at      timestamptz,
    finished_at     timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS step_runs_job_step_idx
    ON media_workflow.step_runs (job_id, step_key);
CREATE INDEX IF NOT EXISTS step_runs_claim_idx
    ON media_workflow.step_runs (status, claimed_at);       -- CLAIMED 超时扫描
CREATE INDEX IF NOT EXISTS step_runs_job_status_idx
    ON media_workflow.step_runs (job_id, status);

COMMIT;
