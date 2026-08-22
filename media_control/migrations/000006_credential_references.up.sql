CREATE TABLE IF NOT EXISTS credential_references (
    id text PRIMARY KEY,
    workspace_id text NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    owner_user_id text NOT NULL,
    name text NOT NULL,
    provider_id text NOT NULL REFERENCES providers(id),
    secret_ref text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT credential_references_status_check CHECK (status IN ('active', 'revoked'))
);

CREATE INDEX IF NOT EXISTS idx_credential_references_owner_workspace_updated
    ON credential_references (owner_user_id, workspace_id, updated_at DESC);
