# media_auth 与旧 control-plane 切换规范

## 1. 目标与边界

本规范定义旧 `agent-platform/control-plane` 将认证所有权切换到 `media_auth` 的运行时契约、数据迁移和回滚方式。认证写模型由 `media_auth` 持有；control-plane 仅保留业务资源及一个按相同用户 id 维护的用户投影，用于历史外键和展示关联。

内部 API 必须配置相同的 `MEDIA_AUTH_INTERNAL_TOKEN`。它们不进入公共 OpenAPI，也不得经公网网关暴露。生产环境不得将该值留空。

## 2. 能力与开关

| 能力 | control-plane 开关 | 关闭时 | 开启时 |
|---|---|---|---|
| `/api/auth` 反代 | `MEDIA_AUTH_PROXY_AUTH` | 旧本地路由 | media_auth |
| `/api/oauth` 反代 | `MEDIA_AUTH_PROXY_OAUTH` | 旧本地路由 | media_auth |
| `/api/service-tokens` 反代 | `MEDIA_AUTH_PROXY_SERVICE_TOKENS` | 旧本地路由 | media_auth |
| `/api/mcp-oauth` 反代 | `MEDIA_AUTH_PROXY_MCP_OAUTH` | 旧本地路由 | media_auth |
| service token 校验 | `MEDIA_AUTH_REMOTE_TOKENS` | 本地表查询 | `/internal/token/verify` |
| session 校验 | `MEDIA_AUTH_REMOTE_SESSIONS` | 本地 JWT 验签 | `/internal/token/session`，即时检查用户状态和 `auth_version` |
| platform token | `MEDIA_AUTH_REMOTE_PLATFORM_TOKENS` | 本地表查询/创建 | `/internal/token/platform` |
| RS256 本地验签 | `MEDIA_AUTH_VERIFY_RS256` | HS256 | JWKS RS256 优先；`JWT_ACCEPT_LEGACY_HS256` 控制旧 token 回退 |

所有开关接受 `1` 或 `true`。远程地址由 `MEDIA_AUTH_URL` 指定。反代保留请求方法、body、cookie、Authorization 和响应的 `Set-Cookie`/重定向信息。`/api/auth/me` 的 `default_prompt_name` 仍由 control-plane 根据 `default_prompt_id` 补齐。

## 3. 数据迁移

迁移程序位于旧 control-plane 的 `scripts/migrate-media-auth.ts`，覆盖 users、identities、service tokens、OAuth 与 MCP OAuth 共 8 张认证表。它只复制源表与目标表的同名列，使用原主键 upsert，默认 dry-run：

```bash
CONTROL_PLANE_DB_URL='postgres://...' \
MEDIA_AUTH_DB_DSN='postgres://...' \
npm run migrate:media-auth

# 备份并确认 dry-run 计数后
npm run migrate:media-auth -- --apply
```

迁移完成后按表核对行数和抽样主键。control-plane 的旧 migration 是历史引导记录，不删除；切换后的运行时认证路径不再将这些表视为权威数据。业务表仍引用用户 id，因此远程 session 校验成功后，control-plane 将 principal JIT upsert 到本地 `users` 投影。

## 4. 灰度顺序

1. 部署 media_auth，完成迁移 dry-run/apply 和数据核对，保持所有 control-plane 开关关闭。
2. 开启四个 proxy 开关，验证登录、OAuth、token 管理与 MCP OAuth。
3. 开启 `MEDIA_AUTH_REMOTE_TOKENS`、`MEDIA_AUTH_REMOTE_PLATFORM_TOKENS`。
4. 开启 `MEDIA_AUTH_REMOTE_SESSIONS`。该模式是当前推荐切换终态，可在软删除或改密后即时拒绝旧 session。
5. 如需降低远程 session 调用，再进入 RS256 双验窗口：media_auth 配置 RSA 私钥/公钥和 `jwt_key_id`，control-plane 开启 `MEDIA_AUTH_VERIFY_RS256`，待旧 HS256 token 过期后关闭 `JWT_ACCEPT_LEGACY_HS256`。在 auth 事件消费者完成前，不用本地缓存替代需要即时撤销的远程校验。

每一步均独立观察错误率、登录成功率和 401/403 分布。回滚时按相反顺序关闭对应开关；数据迁移使用 upsert，可在修复后重跑。

## 5. 验收

- Go HTTP E2E 必须覆盖 internal credential 拒绝、platform token 获取、session introspection 以及软删除后的 session 撤销。
- 实际 `media-cli` 进程必须覆盖登录、用户创建/删除和列表终态，不能只调用 CLI 内部 Go 包。
- control-plane 需要完整单元测试、lint 和 bundle build；迁移脚本必须作为 build entry 编译。
- 前端需要 lint、unit test 和 production build，并验证企微 identity 删除路径。
- 生产切换另需真实数据库迁移演练；本地无旧 control-plane 数据库时，不得宣称已完成该步骤。
