-- 0001_init.down.sql：回滚 0001_init（删除 8 张表 + media_auth schema）。
--
-- 注意：DROP SCHEMA ... CASCADE 会连带删除 schema 下所有表/索引，
-- 仅用于本地/测试回滚；生产回滚应逐表谨慎处理。

BEGIN;

DROP TABLE IF EXISTS media_auth.mcp_oauth_tokens;
DROP TABLE IF EXISTS media_auth.mcp_oauth_clients;
DROP TABLE IF EXISTS media_auth.oauth_refresh_tokens;
DROP TABLE IF EXISTS media_auth.oauth_authorization_codes;
DROP TABLE IF EXISTS media_auth.oauth_clients;
DROP TABLE IF EXISTS media_auth.service_tokens;
DROP TABLE IF EXISTS media_auth.user_identities;
DROP TABLE IF EXISTS media_auth.users;

DROP SCHEMA IF EXISTS media_auth;

COMMIT;
