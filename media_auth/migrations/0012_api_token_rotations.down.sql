BEGIN;
-- Refuse rollback while work is pending; never discard credential cleanup intent.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_auth.api_token_rotations WHERE status IN ('pending','recovering'))
        OR EXISTS (SELECT 1 FROM media_auth.api_tokens WHERE status = 'rotating') THEN
        RAISE EXCEPTION 'pending API token rotations must be resolved before rollback';
    END IF;
END $$;
DROP TABLE media_auth.api_token_rotations;
ALTER TABLE media_auth.api_tokens DROP CONSTRAINT api_tokens_status_check;
ALTER TABLE media_auth.api_tokens ADD CONSTRAINT api_tokens_status_check
    CHECK (status IN ('active','revoking','revoked','expired'));
COMMIT;
