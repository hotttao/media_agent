BEGIN;

ALTER TABLE media_auth.users
    ADD COLUMN IF NOT EXISTS kratos_identity_id text;

CREATE UNIQUE INDEX IF NOT EXISTS users_kratos_identity_id_key
    ON media_auth.users (kratos_identity_id)
    WHERE kratos_identity_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS media_auth.organizations (
    id         text PRIMARY KEY,
    name       text NOT NULL,
    status     text NOT NULL DEFAULT 'active',
    version    bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT organizations_status_check CHECK (status IN ('active', 'disabled'))
);

CREATE TABLE IF NOT EXISTS media_auth.organization_members (
    organization_id text NOT NULL REFERENCES media_auth.organizations(id),
    user_id          text NOT NULL REFERENCES media_auth.users(id),
    role             text NOT NULL,
    joined_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT organization_members_role_check CHECK (role IN ('admin', 'member')),
    PRIMARY KEY (organization_id, user_id)
);

CREATE INDEX IF NOT EXISTS organization_members_user_idx
    ON media_auth.organization_members (user_id);

CREATE TABLE IF NOT EXISTS media_auth.organization_entitlements (
    organization_id text NOT NULL REFERENCES media_auth.organizations(id),
    feature_key      text NOT NULL,
    granted_by       text NOT NULL,
    granted_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, feature_key)
);

-- Preserve installations created before Organization existed. All legacy
-- Teams are placed in one explicit migration organization; existing Team
-- admins become Organization admins and the remaining members become members.
INSERT INTO media_auth.organizations (id, name)
SELECT 'org_legacy', 'Legacy Organization'
WHERE EXISTS (SELECT 1 FROM media_auth.teams)
ON CONFLICT (id) DO NOTHING;

INSERT INTO media_auth.organization_members (organization_id, user_id, role)
SELECT 'org_legacy', tm.user_id,
       CASE WHEN bool_or(tm.role = 'admin') THEN 'admin' ELSE 'member' END
FROM media_auth.team_members tm
GROUP BY tm.user_id
ON CONFLICT (organization_id, user_id) DO UPDATE SET role = EXCLUDED.role;

ALTER TABLE media_auth.teams
    ADD COLUMN IF NOT EXISTS organization_id text REFERENCES media_auth.organizations(id),
    ADD COLUMN IF NOT EXISTS entitlement_mode text NOT NULL DEFAULT 'inherit',
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'active';

UPDATE media_auth.teams SET organization_id = 'org_legacy'
WHERE organization_id IS NULL;

ALTER TABLE media_auth.teams
    ALTER COLUMN organization_id SET NOT NULL;

DO $$ BEGIN
    ALTER TABLE media_auth.teams ADD CONSTRAINT teams_entitlement_mode_check
        CHECK (entitlement_mode IN ('inherit', 'restricted'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    ALTER TABLE media_auth.teams ADD CONSTRAINT teams_status_check
        CHECK (status IN ('active', 'decommissioning', 'decommissioned'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE INDEX IF NOT EXISTS teams_organization_idx
    ON media_auth.teams (organization_id);

CREATE TABLE IF NOT EXISTS media_auth.team_entitlements (
    team_id     text NOT NULL REFERENCES media_auth.teams(id),
    feature_key text NOT NULL,
    granted_by  text NOT NULL,
    granted_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, feature_key)
);

CREATE TABLE IF NOT EXISTS media_auth.authorization_outbox (
    id             text PRIMARY KEY,
    aggregate_type text NOT NULL,
    aggregate_id   text NOT NULL,
    event_type     text NOT NULL,
    payload        jsonb NOT NULL,
    sequence       bigint NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    delivered_at   timestamptz,
    attempts       integer NOT NULL DEFAULT 0,
    last_error     text,
    UNIQUE (aggregate_type, aggregate_id, sequence)
);

CREATE INDEX IF NOT EXISTS authorization_outbox_pending_idx
    ON media_auth.authorization_outbox (created_at)
    WHERE delivered_at IS NULL;

COMMIT;
