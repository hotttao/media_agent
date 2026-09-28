CREATE INDEX api_tokens_revoking_scan_idx
    ON media_auth.api_tokens (id)
    WHERE status = 'revoking';
