BEGIN;
CREATE TABLE media_auth.platform_tokens (
    id text PRIMARY KEY CHECK (id ~ '^[A-Za-z0-9_-]{1,128}$'),
    user_id text NOT NULL REFERENCES media_auth.users(id),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    ciphertext bytea NOT NULL CHECK (octet_length(ciphertext) BETWEEN 17 AND 4096),
    nonce bytea NOT NULL CHECK (octet_length(nonce) = 12),
    key_version text NOT NULL CHECK (key_version ~ '^[A-Za-z0-9_-]{1,64}$'),
    created_at timestamptz NOT NULL,
    expires_at timestamptz,
    revoked_at timestamptz
);
CREATE UNIQUE INDEX platform_tokens_current_user_idx ON media_auth.platform_tokens(user_id) WHERE revoked_at IS NULL;
COMMIT;
