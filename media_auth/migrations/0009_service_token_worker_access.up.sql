BEGIN;

ALTER TABLE media_auth.service_tokens
    ADD COLUMN IF NOT EXISTS organization_id text REFERENCES media_auth.organizations(id),
    ADD COLUMN IF NOT EXISTS scopes text[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS expires_at timestamptz;

CREATE INDEX IF NOT EXISTS idx_service_tokens_organization
    ON media_auth.service_tokens (organization_id)
    WHERE revoked_at IS NULL;

ALTER TABLE media_auth.service_tokens
    ADD CONSTRAINT service_tokens_worker_access_consistent
    CHECK (
        is_platform
        OR cardinality(scopes) = 0
        OR (organization_id IS NOT NULL AND expires_at IS NOT NULL)
    );

COMMIT;
