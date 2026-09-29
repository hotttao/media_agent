BEGIN;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_auth.worker_identities) THEN
        RAISE EXCEPTION 'worker identities exist; refusing to discard identity history';
    END IF;
END $$;
DROP TABLE media_auth.worker_identities;
COMMIT;
