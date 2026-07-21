-- 0004 down: 回退 service_tokens.created_by 为无 CASCADE（NO ACTION）。
BEGIN;

ALTER TABLE media_auth.service_tokens DROP CONSTRAINT IF EXISTS service_tokens_created_by_fkey;
ALTER TABLE media_auth.service_tokens ADD CONSTRAINT service_tokens_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES media_auth.users(id);

COMMIT;
