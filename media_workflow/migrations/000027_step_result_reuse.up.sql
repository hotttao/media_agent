BEGIN;
CREATE TABLE media_workflow.step_result_reuse (
 id TEXT PRIMARY KEY CHECK(id ~ '^reuse_[0-9a-f]{64}$'),
 organization_id TEXT NOT NULL CHECK(organization_id <> ''),
 workspace_id TEXT NOT NULL CHECK(workspace_id <> ''),
 owner_user_id TEXT NOT NULL CHECK(owner_user_id <> ''),
 scope TEXT NOT NULL CHECK(scope <> ''),
 fingerprint TEXT NOT NULL CHECK(fingerprint ~ '^[0-9a-f]{64}$'),
 source_job_id TEXT NOT NULL REFERENCES media_workflow.jobs(id) ON DELETE CASCADE,
 source_step_run_id TEXT NOT NULL REFERENCES media_workflow.step_runs(id) ON DELETE CASCADE,
 source_result_key TEXT NOT NULL CHECK(source_result_key <> ''),
 asset_id TEXT NOT NULL CHECK(asset_id LIKE 'gen\_%' ESCAPE '\'),
 object_path TEXT NOT NULL DEFAULT '',
 content TEXT NOT NULL DEFAULT '',
 asset_kind TEXT NOT NULL CHECK(asset_kind <> ''),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(source_step_run_id, source_result_key),
 CHECK(object_path <> '' OR btrim(content) <> '')
);
CREATE INDEX step_result_reuse_match ON media_workflow.step_result_reuse
 (organization_id,workspace_id,owner_user_id,scope,fingerprint,created_at DESC,id DESC);
CREATE FUNCTION media_workflow.protect_step_result_reuse() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'generation reuse evidence is immutable';
END $$;
CREATE TRIGGER frozen_step_result_reuse BEFORE UPDATE ON media_workflow.step_result_reuse
FOR EACH ROW EXECUTE FUNCTION media_workflow.protect_step_result_reuse();
COMMIT;
