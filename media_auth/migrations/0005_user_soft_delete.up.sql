BEGIN;

ALTER TABLE media_auth.users
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS auth_version bigint NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS disabled_at timestamptz,
    ADD COLUMN IF NOT EXISTS deleted_at timestamptz,
    ADD COLUMN IF NOT EXISTS deleted_by text;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'users_status_check'
          AND conrelid = 'media_auth.users'::regclass
    ) THEN
        ALTER TABLE media_auth.users
            ADD CONSTRAINT users_status_check CHECK (status IN ('active', 'disabled', 'deleted'));
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS users_active_admin_idx
    ON media_auth.users (role, status) WHERE role = 'admin' AND status = 'active';

CREATE TABLE media_auth.user_access_outbox (
    id              text PRIMARY KEY,
    user_id         text NOT NULL REFERENCES media_auth.users(id),
    auth_version    bigint NOT NULL,
    event_type      text NOT NULL DEFAULT 'UserAccessRevoked',
    status          text NOT NULL DEFAULT 'pending',
    attempts        integer NOT NULL DEFAULT 0,
    available_at    timestamptz NOT NULL DEFAULT now(),
    delivered_at    timestamptz,
    last_error      text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, auth_version)
);

CREATE INDEX user_access_outbox_pending_idx
    ON media_auth.user_access_outbox (available_at, created_at)
    WHERE status = 'pending';

COMMIT;
