BEGIN;
ALTER TABLE media_auth.api_tokens DROP CONSTRAINT api_tokens_status_check;
ALTER TABLE media_auth.api_tokens ADD CONSTRAINT api_tokens_status_check
    CHECK (status IN ('active','rotating','revoking','revoked','expired'));
CREATE TABLE media_auth.api_token_rotations (
    id text PRIMARY KEY,
    token_id text NOT NULL REFERENCES media_auth.api_tokens(id),
    old_key_id text NOT NULL,
    actor_id text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending','recovering','completed','aborted')),
    deadline timestamptz NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX api_token_rotations_pending_token_idx
    ON media_auth.api_token_rotations(token_id) WHERE status IN ('pending','recovering');
CREATE INDEX api_token_rotations_recovery_idx
    ON media_auth.api_token_rotations(id) WHERE status IN ('pending','recovering');
COMMIT;
