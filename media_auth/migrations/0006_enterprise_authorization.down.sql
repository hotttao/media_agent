BEGIN;

DROP TABLE IF EXISTS media_auth.authorization_outbox;
DROP TABLE IF EXISTS media_auth.team_entitlements;
DROP INDEX IF EXISTS media_auth.teams_organization_idx;
ALTER TABLE media_auth.teams
    DROP CONSTRAINT IF EXISTS teams_status_check,
    DROP CONSTRAINT IF EXISTS teams_entitlement_mode_check,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS entitlement_mode,
    DROP COLUMN IF EXISTS organization_id;
DROP TABLE IF EXISTS media_auth.organization_entitlements;
DROP TABLE IF EXISTS media_auth.organization_members;
DROP TABLE IF EXISTS media_auth.organizations;
DROP INDEX IF EXISTS media_auth.users_kratos_identity_id_key;
ALTER TABLE media_auth.users DROP COLUMN IF EXISTS kratos_identity_id;

COMMIT;
