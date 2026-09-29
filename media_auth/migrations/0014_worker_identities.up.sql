BEGIN;
CREATE TABLE media_auth.worker_identities (
    id text PRIMARY KEY CHECK (id ~ '^[0-9a-f]{32}$'),
    owner_user_id text NOT NULL REFERENCES media_auth.users(id),
    organization_id text NOT NULL REFERENCES media_auth.organizations(id),
    name text NOT NULL CHECK (name = btrim(name) AND char_length(name) BETWEEN 1 AND 200),
    status text NOT NULL CHECK (status IN ('active','disabled','deleted')),
    created_at timestamptz NOT NULL,
    disabled_at timestamptz,
    deleted_at timestamptz,
    CONSTRAINT worker_identities_status_shape CHECK (
        (status <> 'active' OR (disabled_at IS NULL AND deleted_at IS NULL))
        AND (status <> 'disabled' OR (disabled_at IS NOT NULL AND deleted_at IS NULL))
        AND (status <> 'deleted' OR deleted_at IS NOT NULL)
    )
);
CREATE INDEX worker_identities_owner_idx ON media_auth.worker_identities(owner_user_id,created_at,id);
CREATE INDEX worker_identities_inactive_idx ON media_auth.worker_identities(id) WHERE status <> 'active';
COMMIT;
