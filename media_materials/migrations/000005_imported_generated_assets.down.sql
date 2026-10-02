BEGIN;

DROP INDEX IF EXISTS media_materials.gen_assets_legacy_source_idx;
ALTER TABLE media_materials.generated_assets
  DROP CONSTRAINT IF EXISTS generated_assets_origin_check,
  DROP CONSTRAINT IF EXISTS generated_assets_payload_check;

-- Rollback intentionally refuses if imported assets still exist; they cannot
-- satisfy the old mandatory Job/StepRun/Template columns without fabrication.
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM media_materials.generated_assets WHERE origin = 'legacy_import') THEN
    RAISE EXCEPTION 'cannot roll back imported generated assets while historical rows exist';
  END IF;
END $$;

ALTER TABLE media_materials.generated_assets
  ALTER COLUMN job_id SET NOT NULL,
  ALTER COLUMN step_run_id SET NOT NULL,
  ALTER COLUMN template_id SET NOT NULL,
  ALTER COLUMN template_version SET NOT NULL;
ALTER TABLE media_materials.generated_assets
  ADD CONSTRAINT generated_assets_check CHECK (url IS NOT NULL OR content IS NOT NULL);
ALTER TABLE media_materials.generated_assets
  DROP COLUMN imported_at,
  DROP COLUMN source_created_at,
  DROP COLUMN source_sha256,
  DROP COLUMN source_record_id,
  DROP COLUMN source_system,
  DROP COLUMN object_path,
  DROP COLUMN origin;

COMMIT;
