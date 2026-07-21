-- =====================================================================
-- media_auth - 0004: service_tokens.created_by 加 ON DELETE CASCADE
-- =====================================================================
-- 删 user 时级联清其 service_tokens（含 platform token）。
-- 其余 user 关联表（user_identities / team_members / oauth_authorization_codes /
-- oauth_refresh_tokens / mcp_oauth_tokens）已有 ON DELETE CASCADE，仅 service_tokens 缺。
-- 改后 repo.DeleteUser = 单条 DELETE，全表级联清理。
-- =====================================================================

BEGIN;

ALTER TABLE media_auth.service_tokens DROP CONSTRAINT IF EXISTS service_tokens_created_by_fkey;
ALTER TABLE media_auth.service_tokens ADD CONSTRAINT service_tokens_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES media_auth.users(id) ON DELETE CASCADE;

COMMIT;
