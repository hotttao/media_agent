BEGIN;

CREATE TABLE media_auth.api_tokens (
    id                text PRIMARY KEY,
    talos_key_id      text NOT NULL UNIQUE,
    subject_user_id   text REFERENCES media_auth.users(id),
    actor_type        text NOT NULL CHECK (actor_type IN ('user','agent','personal_worker','service')),
    actor_id          text NOT NULL,
    organization_id   text REFERENCES media_auth.organizations(id),
    name              text NOT NULL,
    scopes            text[] NOT NULL DEFAULT '{}',
    status            text NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoking','revoked','expired')),
    expires_at        timestamptz,
    created_by        text NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    rotated_at        timestamptz,
    revoked_at        timestamptz,
    CHECK (
        (actor_type = 'service' AND subject_user_id IS NULL)
        OR
        (actor_type <> 'service' AND subject_user_id IS NOT NULL)
    )
);

CREATE INDEX api_tokens_subject_idx
    ON media_auth.api_tokens (subject_user_id, created_at DESC);
CREATE INDEX api_tokens_actor_idx
    ON media_auth.api_tokens (actor_type, actor_id)
    WHERE status = 'active';
CREATE INDEX api_tokens_organization_idx
    ON media_auth.api_tokens (organization_id)
    WHERE status = 'active';

COMMIT;
