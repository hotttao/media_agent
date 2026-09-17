BEGIN;
DROP TABLE IF EXISTS media_auth.authorization_projection_checkpoints;
DROP INDEX IF EXISTS media_auth.authorization_outbox_claim_idx;
ALTER TABLE media_auth.authorization_outbox
    DROP COLUMN IF EXISTS claimed_at,
    DROP COLUMN IF EXISTS next_attempt_at;
COMMIT;
