-- =====================================================================
-- media_auth — OAuth client seed（media_materials + media_workflow）
-- =====================================================================
-- 两个机密 client（confidential，有 secret_hash），PKCE S256。
-- created_by 用现有 admin 用户 u_smoke。
-- client_secret 明文走 config（环境变量 MW_OAUTH_CLIENT_SECRET）。
-- 部署时改 secret 后 UPDATE secret_hash。
-- =====================================================================

BEGIN;

-- 0003 依赖 admin 用户 u_smoke（作为 oauth_clients.created_by）。
-- 全新 DB（含 e2e 隔离库）无此 user 会触发 FK 违规，故先 seed（已存在则跳过）。
-- dev/prod 若已有 u_smoke，ON CONFLICT DO NOTHING 为 no-op。
INSERT INTO media_auth.users (id, username, display_name, role)
VALUES ('u_smoke', 'smoke', 'Smoke User', 'admin')
ON CONFLICT (id) DO NOTHING;

-- media_materials client
INSERT INTO media_auth.oauth_clients (id, name, secret_hash, redirect_uris, homepage_url, description, created_by, created_at, updated_at)
VALUES (
    'media_materials',
    'Media Materials',
    '6aea7acac963d65be28677e508c567c345d7f9fc16b418a274354e7a7664af96',  -- SHA-256("mw_materials_secret_dev")
    '{"http://127.0.0.1:8003/api/auth/callback"}',
    'http://127.0.0.1:8003',
    'media_materials 服务 OAuth client',
    'u_smoke',
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    secret_hash = EXCLUDED.secret_hash,
    redirect_uris = EXCLUDED.redirect_uris,
    updated_at = now();

-- media_workflow client
INSERT INTO media_auth.oauth_clients (id, name, secret_hash, redirect_uris, homepage_url, description, created_by, created_at, updated_at)
VALUES (
    'media_workflow',
    'Media Workflow',
    'e7fefa8fde42bb07d0a7ad2643c9ef2dfc52230774cfb085bbf2b9a3fc60e30b',  -- SHA-256("mw_workflow_secret_dev")
    '{"http://127.0.0.1:8002/api/auth/callback"}',
    'http://127.0.0.1:8002',
    'media_workflow 服务 OAuth client',
    'u_smoke',
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    secret_hash = EXCLUDED.secret_hash,
    redirect_uris = EXCLUDED.redirect_uris,
    updated_at = now();

COMMIT;
