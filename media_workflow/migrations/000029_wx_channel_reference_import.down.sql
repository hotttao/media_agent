-- Published acquisition contracts and frozen jobs may reference this capability.
-- Do not silently delete or invalidate them during rollback.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM media_workflow.execution_capabilities WHERE capability_key = 'wx_channel.download.record') THEN
  RAISE EXCEPTION 'wx_channel published contract must be retained; roll forward or restore a pre-migration backup';
 END IF;
END $$;
