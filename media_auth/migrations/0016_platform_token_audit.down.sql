BEGIN;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_auth.platform_token_audit_outbox) THEN
        RAISE EXCEPTION 'platform token audit history exists; refusing to discard';
    END IF;
END $$;
DROP TABLE media_auth.platform_token_audit_outbox;
COMMIT;
