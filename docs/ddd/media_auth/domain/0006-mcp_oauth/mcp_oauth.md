---
name: mcp_oauth
description: 外部 MCP OAuth 授权状态与加密凭据托管
dependencies:
    - auth
related_workflow:
    - mcp_oauth_connect
---

## 一句话职责
mcp_oauth 负责外部 MCP 的 OAuth Discovery、授权回调、加密凭据托管、刷新与撤销；Plugin 负责 MCP Definition、Workspace Binding、连接投影和外部 MCP 请求代理。

## 边界

### 做什么
- 探测给定 MCP server URL 是否需要 OAuth（拉 `/.well-known/oauth-authorization-server`）。
- 缓存 MCP server 的 OAuth metadata + 动态注册的 client（`mcp_oauth_clients`，以 `server_origin` 为主键）。
- 发起授权：生成 PKCE S256 + state，重定向到第三方 authorize endpoint。
- 回调：换 token，按 `(user_id, server_origin)` upsert 存 access/refresh token，postMessage 通知 opener。
- 批量查询用户对多 MCP server 的连接状态。
- 刷新 access_token（含 forceRefresh）；刷新失败宽限（见 Refresh Failure Grace）。
- 断开：删除用户对某 server 的 token。

### 不做什么
- 不做 media_auth 作为 Authorization Server（角色相反，归 [oauth](../0005-oauth/oauth.md)）。
- 不解析 token 内容；access/refresh token 对业务语义不透明并加密保存。
- 不负责 MCP 协议消息转发（归 `media_plugin` External OAuth MCP Proxy）。
- 不在 HTTP 层暴露 client 注册 CRUD（动态注册内部完成）。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 发现 OAuth | 探测 MCP server 是否需 OAuth | `DiscoverOAuth` `POST /api/mcp-oauth/discover` |
| 连接状态 | 批量查询用户对各 server 的连接 | `GetConnectionStatus` `GET /api/mcp-oauth/status` |
| 发起授权 | 重定向到第三方 authorize（popup） | `StartAuthorization` `GET /api/mcp-oauth/authorize` |
| 回调换 token | 换 token 存库，返回 HTML postMessage | `HandleCallback` `GET /api/mcp-oauth/callback` |
| 断开 | 删除用户对该 server 的 token | `Disconnect` `DELETE /api/mcp-oauth/:server_origin` |
| 取有效 token（内部） | 仅供 allowlist 中的 media_plugin Proxy 取得短暂可用 access token | `GetUsableCredential(credentialRef)` |

## 数据模型

### Tables — `mcp_oauth_clients`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| server_origin | text PK | MCP server origin |
| metadata | jsonb | 缓存的 OAuth metadata（authorize/token/registration endpoint） |
| client_id | text | 动态注册所得 client_id |
| client_secret_ciphertext | bytea nullable | 动态注册所得 secret 的 AEAD 密文 |
| nonce | bytea nullable | client secret 加密 nonce |
| key_version | text nullable | 加密密钥版本 |
| created_at / updated_at | timestamptz | 时间 |

### Tables — `mcp_oauth_authorization_intents`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| state_hash | bytea PK | 随机 state 的 SHA-256 Hash |
| user_id | text | 发起授权的用户 |
| workspace_id | text | 发起授权的 Workspace |
| server_origin | text | 外部 MCP Origin |
| code_verifier_ciphertext | bytea | PKCE verifier 的 AEAD 密文 |
| nonce | bytea | 加密 nonce |
| key_version | text | 加密密钥版本 |
| expires_at | timestamptz | 过期时间 |
| consumed_at | timestamptz nullable | callback 原子消费时间 |
| created_at | timestamptz | 创建时间 |

### Tables — `mcp_oauth_tokens`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| user_id | text | 用户；关联 `users (CASCADE)`（复合 PK） |
| server_origin | text | MCP server；关联 `mcp_oauth_clients (CASCADE)`（复合 PK） |
| access_token_ciphertext | bytea | 第三方 access token 密文 |
| refresh_token_ciphertext | bytea | 第三方 refresh token 密文 |
| nonce | bytea | AEAD nonce |
| key_version | text | 密钥版本 |
| token_type | text | 默认 Bearer |
| scope | text | scope |
| expires_at | timestamptz | access 过期 |
| refresh_fail_count | integer | 刷新失败次数 |
| refresh_fail_first_at | timestamptz | 首次失败时间 |
| created_at / updated_at | timestamptz | 时间 |

### UUID 生成规则
无独立 id；PK `(user_id, server_origin)`。`code_verifier` = base64url(32 bytes)，`code_challenge` = base64url(sha256(verifier))（S256）；`state` = base64url(16 bytes)。

## 能力详细说明

### DiscoverOAuth
```
1. 入参：url
2. GET {origin}/.well-known/oauth-authorization-server (header MCP-Protocol-Version: 2025-03-26, 5s timeout)
3. 无 metadata / 缺 authorization_endpoint+token_endpoint → { oauth_required: false }
4. 有 → upsert mcp_oauth_clients(server_origin, metadata) → { oauth_required: true, server_origin }
5. 上游不可达 → 502
```

### GetConnectionStatus
```
1. 入参：servers（逗号分隔 server_origin 列表）
2. 逐个查 mcp_oauth_tokens(user_id=session.sub, server_origin)
3. connected = 存在且有效（未过宽限）；expires_at 返回
4. 返回 map[server_origin]{ connected, expires_at? }
```

### StartAuthorization
```
1. 入参：server_origin, workspace_id
2. 缺参 → 400；未 discover → 400
3. code_verifier = rand(32); code_challenge = S256; state = rand(16)
4. 持久化 `state_hash`、加密 `code_verifier`、user/workspace/origin 与 10 分钟过期时间，支持多副本 callback
5. c.Redirect 到 {authorize_endpoint}?...&code_challenge=&state=
```

### HandleCallback
```
1. 入参：code, state, error?
2. 缺 code/state → 400；按 state_hash 查询授权意图，不存在、过期或已消费 → 400
3. pending.user_id != session.sub → 403
4. 换 token：POST {token_endpoint} (code, code_verifier, redirect_uri) → access/refresh/expires_in
5. upsertToken(user_id, server_origin, tokens)：ON CONFLICT DO UPDATE，重置 refresh_fail_count=0 / refresh_fail_first_at=NULL
   - expires_at = now + expires_in（无 expires_in 则 +1800s）
6. 通过 Outbox 发布非敏感 Credential 状态变化，由 Plugin 更新 Connection 与 Snapshot
7. 返回 HTML：window.opener.postMessage(...) + window.close()
```

### getValidAccessToken（内部）
```
1. 取 token 行；expires_at 为空则回退 updated_at+1800s
2. 未过期且非 forceRefresh → 返回 access_token
3. 过期/forceRefresh → 调 {token_endpoint} refresh
   - 成功 → upsert（重置 fail 计数）→ 返回新 access_token
   - 失败（invalid_grant/invalid_client/unauthorized_client）→ 进入宽限判定：
     refresh_fail_count++；首次失败记 refresh_fail_first_at
     若 count>=3 且 first_at 距今>=600s → 删 token 行，抛 McpOAuthReauthRequired
     否则（宽限窗口内）→ 返回 nil（视为暂态）
```

### Disconnect
```
1. 入参：server_origin（path）
2. DELETE mcp_oauth_tokens WHERE user_id=sub AND server_origin=?
3. 返回 success
```

## 关联关系

### 依赖的 domain
- [auth](../0001-auth/auth.md) — 复用 session.sub 作为 user_id

### 参与的 workflow
- [mcp_oauth_connect](../../workflow/0003-mcp-oauth-connect/mcp_oauth_connect.md) — 作为 OAuth client 主驱动

### 关联术语
[MCP OAuth Client](../../CONTEXT.md)、[MCP OAuth Token](../../CONTEXT.md)、[Refresh Failure Grace](../../CONTEXT.md)

## Plugin 可用凭据端口

POST `/internal/mcp-oauth/token` 仅接受配置的 `X-MCP-Plugin-Credential`，与浏览器 Session 和其他内部服务令牌隔离。Plugin 在验证固定 Binding、Workspace 与目录来源后提交 user_id 和规范化 server_origin。Auth 返回当前有效 access_token，必要时沿用既有 refresh 流程；不返回 refresh_token，响应 no-store。缺少连接返回 needs_reauth；服务故障返回503且不暴露凭据。OAuth 仓储禁止渲染带凭据参数的 SQL 日志。
