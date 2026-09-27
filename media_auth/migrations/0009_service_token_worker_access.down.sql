BEGIN;

ALTER TABLE media_auth.service_tokens
    DROP CONSTRAINT IF EXISTS service_tokens_worker_access_consistent;
DROP INDEX IF EXISTS media_auth.idx_service_tokens_organization;
ALTER TABLE media_auth.service_tokens
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS scopes,
    DROP COLUMN IF EXISTS organization_id;

COMMIT;
