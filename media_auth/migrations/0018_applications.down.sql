BEGIN;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM media_auth.users WHERE managed_by IS NOT NULL) THEN
  RAISE EXCEPTION 'cannot remove application identity boundary while managed users exist';
 END IF;
END $$;
DROP TABLE media_auth.application_access_credentials;
ALTER TABLE media_auth.users DROP CONSTRAINT users_managed_identity_check;
ALTER TABLE media_auth.users DROP COLUMN managed_by;
DROP TABLE media_auth.applications;
COMMIT;
