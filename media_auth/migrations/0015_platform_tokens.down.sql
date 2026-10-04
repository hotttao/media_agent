BEGIN;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_auth.platform_tokens) THEN
        RAISE EXCEPTION 'platform token history exists; refusing to discard encrypted custody';
    END IF;
END $$;
DROP TABLE media_auth.platform_tokens;
COMMIT;
