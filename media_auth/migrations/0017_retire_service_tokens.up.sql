BEGIN;
LOCK TABLE media_auth.service_tokens IN ACCESS EXCLUSIVE MODE;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM media_auth.service_tokens WHERE is_platform) THEN
        RAISE EXCEPTION 'platform custody migration is required before credential retirement';
    END IF;
END;
$$;

-- Preserve all historical facts; even zero-row writes and TRUNCATE are denied.
CREATE FUNCTION media_auth.reject_retired_service_token_write()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'retired credential archive is read-only';
END;
$$;
CREATE TRIGGER service_tokens_read_only_archive
    BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON media_auth.service_tokens
    FOR EACH STATEMENT EXECUTE FUNCTION media_auth.reject_retired_service_token_write();
ALTER TABLE media_auth.service_tokens ENABLE ALWAYS TRIGGER service_tokens_read_only_archive;
COMMENT ON TABLE media_auth.service_tokens IS
    'Retired read-only credential history. Not an authentication source. Platform facts migrated to encrypted custody.';
COMMIT;
