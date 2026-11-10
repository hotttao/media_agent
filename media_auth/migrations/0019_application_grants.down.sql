BEGIN;
UPDATE media_auth.applications SET credential_version=credential_version+1;
DELETE FROM media_auth.application_access_credentials WHERE team_id<>'';
DROP TABLE media_auth.application_team_grants;
ALTER TABLE media_auth.application_access_credentials DROP COLUMN team_id,DROP COLUMN organization_id,DROP COLUMN scopes;
COMMIT;
