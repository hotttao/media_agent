BEGIN;

DELETE FROM media_auth.authorization_outbox WHERE aggregate_type = 'team';
DELETE FROM media_auth.authorization_projection_checkpoints WHERE aggregate_type = 'team';
ALTER TABLE media_auth.team_entitlements
    DROP CONSTRAINT IF EXISTS team_entitlements_team_id_fkey;
ALTER TABLE media_auth.team_entitlements
    ADD CONSTRAINT team_entitlements_team_id_fkey
    FOREIGN KEY (team_id) REFERENCES media_auth.teams(id);
ALTER TABLE media_auth.teams DROP COLUMN IF EXISTS authorization_version;

COMMIT;
