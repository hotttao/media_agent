BEGIN;
DROP TABLE IF EXISTS media_auth.user_access_outbox;
DROP INDEX IF EXISTS media_auth.users_active_admin_idx;
ALTER TABLE media_auth.users
    DROP CONSTRAINT IF EXISTS users_status_check,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS disabled_at,
    DROP COLUMN IF EXISTS auth_version,
    DROP COLUMN IF EXISTS status;
COMMIT;
