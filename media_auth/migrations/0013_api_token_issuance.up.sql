BEGIN;
ALTER TABLE media_auth.api_tokens ALTER COLUMN talos_key_id DROP NOT NULL;
ALTER TABLE media_auth.api_tokens ADD COLUMN issuance_deadline timestamptz;
ALTER TABLE media_auth.api_tokens ADD COLUMN client_request_key text;
ALTER TABLE media_auth.api_tokens DROP CONSTRAINT api_tokens_status_check;
ALTER TABLE media_auth.api_tokens ADD CONSTRAINT api_tokens_status_check
    CHECK (status IN ('issuing','issuance_cleanup','active','rotating','revoking','revoked','expired'));
ALTER TABLE media_auth.api_tokens ADD CONSTRAINT api_tokens_issuance_shape_check CHECK (
    (talos_key_id IS NULL OR length(talos_key_id) > 0)
    AND (status IN ('issuing','issuance_cleanup') OR talos_key_id IS NOT NULL)
    AND (status NOT IN ('issuing','issuance_cleanup') OR issuance_deadline IS NOT NULL)
    AND (status <> 'issuing' OR talos_key_id IS NULL)
    AND (client_request_key IS NULL OR length(client_request_key) BETWEEN 1 AND 200)
);
CREATE UNIQUE INDEX api_tokens_client_request_idx
    ON media_auth.api_tokens(created_by, client_request_key)
    WHERE client_request_key IS NOT NULL AND status NOT IN ('revoked','expired');
CREATE INDEX api_tokens_issuance_recovery_idx ON media_auth.api_tokens(id)
    WHERE status IN ('issuing','issuance_cleanup');
COMMIT;
