-- Operator-only data cleanup after verified backup and migration dataset reset.
-- Keep all Provider contracts; only the seven known, now-unreferenced fixture keys.
BEGIN;
SET LOCAL lock_timeout='10s';
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM media_workflow.templates
    WHERE capability_key IN ('media.image.model','media.image.style','media.image.first_frame',
      'media.video.generate','media.video.clip','media.content.ai_fill','media.publish.feishu')
  ) THEN RAISE EXCEPTION 'legacy fixture capability is still referenced'; END IF;
END $$;
ALTER TABLE media_workflow.execution_capabilities DISABLE TRIGGER protect_provider_capability_contract;
DELETE FROM media_workflow.execution_capabilities WHERE provider='client' AND version=1 AND
  capability_key IN ('media.image.model','media.image.style','media.image.first_frame',
    'media.video.generate','media.video.clip','media.content.ai_fill','media.publish.feishu');
ALTER TABLE media_workflow.execution_capabilities ENABLE TRIGGER protect_provider_capability_contract;
COMMIT;
