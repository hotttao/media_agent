BEGIN;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_auth.api_tokens WHERE status IN ('issuing','issuance_cleanup') OR talos_key_id IS NULL) THEN
        RAISE EXCEPTION 'pending API token issuances must be resolved before rollback';
    END IF;
END $$;
DROP INDEX media_auth.api_tokens_issuance_recovery_idx;
DROP INDEX media_auth.api_tokens_client_request_idx;
ALTER TABLE media_auth.api_tokens DROP CONSTRAINT api_tokens_issuance_shape_check;
ALTER TABLE media_auth.api_tokens DROP CONSTRAINT api_tokens_status_check;
ALTER TABLE media_auth.api_tokens ADD CONSTRAINT api_tokens_status_check
    CHECK (status IN ('active','rotating','revoking','revoked','expired'));
ALTER TABLE media_auth.api_tokens ALTER COLUMN talos_key_id SET NOT NULL;
ALTER TABLE media_auth.api_tokens DROP COLUMN issuance_deadline;
ALTER TABLE media_auth.api_tokens DROP COLUMN client_request_key;
COMMIT;
