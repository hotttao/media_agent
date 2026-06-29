-- =====================================================================
-- media_auth — 初始化 schema
-- =====================================================================
-- 范围:media_auth 服务自管的全部表(用户、外部身份、Service Token、
--      OAuth Provider 三表、MCP OAuth 两表)。
-- 演进:本文件是 schema 唯一源;后续迁移以 0002_xxx.sql 命名追加。
-- 来源:控制面 migrations/001_init.sql 中的认证相关段 +
--      migrations/117_*.sql 的列补齐(refresh_fail_count + refresh_fail_first_at)。
-- =====================================================================

BEGIN;

-- ─────────────────────────────────────────────────────────────────────
-- 1. users —— 账号主表
-- ─────────────────────────────────────────────────────────────────────
-- id 由应用层 generateId() 产生(短随机字符串),非自增。
-- role: 'user' | 'admin' | 'system'(system 不允许登录,见 auth.proto Login)
-- password_hash: argon2 哈希;空 = LDAP / 企微派生账号,不可改密
-- default_prompt_id / auto_evolution: 用户偏好
-- last_login_at: 仅本地账号登录时更新
-- ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS media_auth.users (
    id                  text PRIMARY KEY,
    username            text NOT NULL,
    display_name        text NOT NULL,
    email               text,
    password_hash       text,
    role                text NOT NULL DEFAULT 'user',
    default_prompt_id   text,
    auto_evolution      boolean NOT NULL DEFAULT false,
    created_at          timestamptz DEFAULT now(),
    last_login_at       timestamptz
);

-- username 唯一(LDAP 派生账号也用 ldap attr 的 username 列此处)
CREATE UNIQUE INDEX IF NOT EXISTS users_username_key
    ON media_auth.users (username);

-- ─────────────────────────────────────────────────────────────────────
-- 2. user_identities —— 外部身份绑定(provider, external_id)
-- ─────────────────────────────────────────────────────────────────────
-- 一用户对同一 provider 至多一条绑定;(provider, external_id) 全局唯一。
-- metadata jsonb 留给 provider-specific 字段(企微 avatar / email 等)。
-- ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS media_auth.user_identities (
    user_id       text NOT NULL REFERENCES media_auth.users(id) ON DELETE CASCADE,
    provider      text NOT NULL,
    external_id   text NOT NULL,
    display_name  text,
    metadata      jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at    timestamptz DEFAULT now(),
    PRIMARY KEY (provider, external_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS user_identities_user_provider_key
    ON media_auth.user_identities (user_id, provider);

CREATE INDEX IF NOT EXISTS user_identities_user_idx
    ON media_auth.user_identities (user_id);

-- ─────────────────────────────────────────────────────────────────────
-- 3. service_tokens —— B2B API token(tos_xxx)
-- ─────────────────────────────────────────────────────────────────────
-- token_hash: SHA-256(token),DB 不存明文
-- token: 仅 platform token 落明文(getPlatformToken 用);非 platform 必为 NULL
-- is_platform: 系统级 platform 令牌;登录成功后自动 ensurePlatformToken
-- 撤销语义:revoked_at 非空 = 失效,GET 列表过滤 revoked_at IS NULL
-- ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS media_auth.service_tokens (
    id          text PRIMARY KEY,
    name        text NOT NULL,
    token_hash  text NOT NULL,
    token       text,                          -- 仅 platform 落明文
    created_by  text REFERENCES media_auth.users(id),
    created_at  timestamptz DEFAULT now(),
    revoked_at  timestamptz,
    is_platform boolean NOT NULL DEFAULT false
);

-- 每用户至多一个 active platform token
CREATE UNIQUE INDEX IF NOT EXISTS idx_service_tokens_platform_user
    ON media_auth.service_tokens (created_by)
    WHERE is_platform = true AND revoked_at IS NULL;

-- ─────────────────────────────────────────────────────────────────────
-- 4. oauth_clients —— OAuth2 第三方客户端注册
-- ─────────────────────────────────────────────────────────────────────
-- redirect_uris: text[],逐元素精确匹配(OAuth2 §3.1.2)
-- secret_hash: SHA-256(client_secret);空 = 公开 client(无需 secret)
-- homepage_url / description: consent 页展示用
-- ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS media_auth.oauth_clients (
    id              text PRIMARY KEY,
    name            text NOT NULL,
    secret_hash     text,
    redirect_uris   text[] NOT NULL,
    homepage_url    text,
    description     text,
    created_by      text NOT NULL REFERENCES media_auth.users(id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- ─────────────────────────────────────────────────────────────────────
-- 5. oauth_authorization_codes —— 一次性 authorization_code
-- ─────────────────────────────────────────────────────────────────────
-- expires_at 10 分钟;消费即 DELETE(防重放)。
-- code_challenge 留 PKCE S256;非空时消费端必须算 code_verifier 校验。
-- ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS media_auth.oauth_authorization_codes (
    code            text PRIMARY KEY,
    client_id       text NOT NULL REFERENCES media_auth.oauth_clients(id) ON DELETE CASCADE,
    user_id         text NOT NULL REFERENCES media_auth.users(id) ON DELETE CASCADE,
    redirect_uri    text NOT NULL,
    code_challenge  text,
    scope           text NOT NULL DEFAULT 'profile',
    expires_at      timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS oauth_authorization_codes_expires_idx
    ON media_auth.oauth_authorization_codes (expires_at);

-- ─────────────────────────────────────────────────────────────────────
-- 6. oauth_refresh_tokens —— 刷新令牌(SHA-256 哈希)
-- ─────────────────────────────────────────────────────────────────────
-- 30 天 TTL;消费即 DELETE(轮转);handler 每小时清理过期。
-- ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS media_auth.oauth_refresh_tokens (
    token_hash  text PRIMARY KEY,
    client_id   text NOT NULL REFERENCES media_auth.oauth_clients(id) ON DELETE CASCADE,
    user_id     text NOT NULL REFERENCES media_auth.users(id) ON DELETE CASCADE,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS oauth_refresh_tokens_expires_idx
    ON media_auth.oauth_refresh_tokens (expires_at);

-- ─────────────────────────────────────────────────────────────────────
-- 7. mcp_oauth_clients —— 第三方 MCP 服务器的 OAuth client 注册
-- ─────────────────────────────────────────────────────────────────────
-- server_origin 作主键(同一 MCP server 仅一个 client)。
-- metadata jsonb 缓存 /.well-known/oauth-authorization-server 响应。
-- ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS media_auth.mcp_oauth_clients (
    server_origin text PRIMARY KEY,
    metadata      jsonb NOT NULL DEFAULT '{}'::jsonb,
    client_id     text NOT NULL,
    client_secret text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- ─────────────────────────────────────────────────────────────────────
-- 8. mcp_oauth_tokens —— 当前用户对每个 MCP 服务器的 token
-- ─────────────────────────────────────────────────────────────────────
-- refresh 失败宽限(对齐控制面 migrations/117 实现):
--   refresh_fail_count      : 连续刷新失败次数
--   refresh_fail_first_at   : 首次失败时间
--   算法: refresh_fail_count >= 3 且 refresh_fail_first_at 距今 >= 600s 才删 token;
--         之前视为暂态(getValidAccessToken 返回 nil);成功 refresh 重置两者。
-- ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS media_auth.mcp_oauth_tokens (
    user_id                   text NOT NULL REFERENCES media_auth.users(id) ON DELETE CASCADE,
    server_origin             text NOT NULL REFERENCES media_auth.mcp_oauth_clients(server_origin) ON DELETE CASCADE,
    access_token              text NOT NULL,
    refresh_token             text,
    token_type                text NOT NULL DEFAULT 'Bearer',
    scope                     text,
    expires_at                timestamptz,
    refresh_fail_count        integer NOT NULL DEFAULT 0,
    refresh_fail_first_at     timestamptz,
    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, server_origin)
);

CREATE INDEX IF NOT EXISTS mcp_oauth_tokens_user_idx
    ON media_auth.mcp_oauth_tokens (user_id);

COMMIT;