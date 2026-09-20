BEGIN;

ALTER TABLE media_auth.teams
    ADD COLUMN IF NOT EXISTS authorization_version bigint NOT NULL DEFAULT 1;

-- Team is the aggregate root; entitlement rows must not prevent aggregate
-- deletion. Migration 0006 created this foreign key without ON DELETE CASCADE.
ALTER TABLE media_auth.team_entitlements
    DROP CONSTRAINT IF EXISTS team_entitlements_team_id_fkey;
ALTER TABLE media_auth.team_entitlements
    ADD CONSTRAINT team_entitlements_team_id_fkey
    FOREIGN KEY (team_id) REFERENCES media_auth.teams(id) ON DELETE CASCADE;

INSERT INTO media_auth.authorization_outbox
    (id, aggregate_type, aggregate_id, event_type, payload, sequence)
SELECT
    'team_bootstrap_' || md5(t.id),
    'team',
    t.id,
    'TeamAuthorizationSnapshotRequested',
    jsonb_build_object('team_id', t.id),
    t.authorization_version
FROM media_auth.teams t
ON CONFLICT (aggregate_type, aggregate_id, sequence) DO NOTHING;

COMMIT;
