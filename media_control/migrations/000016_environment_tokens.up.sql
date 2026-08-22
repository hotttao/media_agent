CREATE TABLE IF NOT EXISTS environment_tokens (
    id text PRIMARY KEY,
    environment_id text NOT NULL REFERENCES runtime_environments(id) ON DELETE CASCADE,
    name text NOT NULL,
    token_hash text NOT NULL,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL,
    revoked_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_environment_tokens_active_hash
    ON environment_tokens (token_hash) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_environment_tokens_environment
    ON environment_tokens (environment_id, created_at DESC);
