BEGIN;

ALTER TABLE media_materials.generated_assets
  ADD COLUMN origin text NOT NULL DEFAULT 'native',
  ADD COLUMN object_path text,
  ADD COLUMN source_system text,
  ADD COLUMN source_record_id text,
  ADD COLUMN source_sha256 text,
  ADD COLUMN source_created_at timestamptz,
  ADD COLUMN imported_at timestamptz;

ALTER TABLE media_materials.generated_assets
  ALTER COLUMN job_id DROP NOT NULL,
  ALTER COLUMN step_run_id DROP NOT NULL,
  ALTER COLUMN template_id DROP NOT NULL,
  ALTER COLUMN template_version DROP NOT NULL;

ALTER TABLE media_materials.generated_assets
  DROP CONSTRAINT IF EXISTS generated_assets_check;
ALTER TABLE media_materials.generated_assets
  ADD CONSTRAINT generated_assets_payload_check
  CHECK (url IS NOT NULL OR content IS NOT NULL OR object_path IS NOT NULL),
  ADD CONSTRAINT generated_assets_origin_check
  CHECK (
    (origin = 'native' AND job_id IS NOT NULL AND step_run_id IS NOT NULL AND template_id IS NOT NULL AND template_version IS NOT NULL AND source_system IS NULL AND source_record_id IS NULL)
    OR
    (origin = 'legacy_import' AND job_id IS NULL AND step_run_id IS NULL AND template_id IS NULL AND template_version IS NULL AND object_path IS NOT NULL AND source_system IS NOT NULL AND source_record_id IS NOT NULL AND source_sha256 ~ '^[0-9a-f]{64}$' AND source_created_at IS NOT NULL AND imported_at IS NOT NULL)
  );

CREATE UNIQUE INDEX gen_assets_legacy_source_idx
  ON media_materials.generated_assets (source_system, source_record_id)
  WHERE source_system IS NOT NULL;

COMMIT;
