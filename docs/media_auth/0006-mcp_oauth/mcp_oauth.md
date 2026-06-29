---
name: mcp_oauth
description: media_auth 反向作为 OAuth client 连接第三方 MCP 服务器的发现/授权/回调/断开
dependencies:
    - auth
related_workflow:
    - mcp_oauth_connect
---

## 一句话职责
mcp_oauth 负责 media_auth 反向作为 OAuth client 连接第三方 MCP 服务器：发现 OAuth metadata、动态注册 client、发起授权 popup、回调换 token 并按用户存储、刷新与失败宽限、断开连接。

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
- 不解析 token 内容（access/refresh token 对 media_auth 不透明，明文存）。
- 不负责 MCP 协议本身的消息转发（归 control_plane mcp-proxy）。
- 不在 HTTP 层暴露 client 注册 CRUD（动态注册内部完成）。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 发现 OAuth | 探测 MCP server 是否需 OAuth | `DiscoverOAuth` `POST /api/mcp-oauth/discover` |
| 连接状态 | 批量查询用户对各 server 的连接 | `GetConnectionStatus` `GET /api/mcp-oauth/status` |
| 发起授权 | 重定向到第三方 authorize（popup） | `StartAuthorization` `GET /api/mcp-oauth/authorize` |
| 回调换 token | 换 token 存库，返回 HTML postMessage | `HandleCallback` `GET /api/mcp-oauth/callback` |
| 断开 | 删除用户对该 server 的 token | `Disconnect` `DELETE /api/mcp-oauth/:server_origin` |
| 取有效 token（内部） | 供 control_plane mcp-proxy 取 access_token（含刷新） | `getValidAccessToken(userID, serverOrigin, forceRefresh?)` |

## 数据模型

### Tables — `mcp_oauth_clients`

| 字段 | 描述 | 类型 | 关联表 | 关联说明 |
|:---|:---|:---|:---|:---|
| server_origin | MCP server origin | text PK | — | — |
| metadata | 缓存的 OAuth metadata（authorize/token/registration endpoint） | jsonb | — | — |
| client_id | 动态注册所得 client_id | text | — | — |
| client_secret | 动态注册所得 secret（原值，可空） | text | — | — |
| created_at / updated_at | 时间 | timestamptz | — | — |

### Tables — `mcp_oauth_tokens`

| 字段 | 描述 | 类型 | 关联表 | 关联说明 |
|:---|:---|:---|:---|:---|
| user_id | 用户 | text | users (CASCADE) | 复合 PK |
| server_origin | MCP server | text | mcp_oauth_clients (CASCADE) | 复合 PK |
| access_token | 第三方 access token（明文） | text | — | 对 media_auth 不透明 |
| refresh_token | 第三方 refresh token | text | — | 可空 |
| token_type | 默认 Bearer | text | — | — |
| scope | scope | text | — | 可空 |
| expires_at | access 过期 | timestamptz | — | 可空，回退 updated_at+1800s |
| refresh_fail_count | 刷新失败次数 | integer | — | 默认 0 |
| refresh_fail_first_at | 首次失败时间 | timestamptz | — | 可空 |
| created_at / updated_at | 时间 | timestamptz | — | — |

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
4. pendingAuths[state] = { user_id=sub, server_origin, code_verifier, workspace_id, created_at=now }（10min TTL，内存）
5. c.Redirect 到 {authorize_endpoint}?...&code_challenge=&state=
```

### HandleCallback
```
1. 入参：code, state, error?
2. 缺 code/state → 400；pendingAuths[state] 不存在/过期 → 400
3. pending.user_id != session.sub → 403
4. 换 token：POST {token_endpoint} (code, code_verifier, redirect_uri) → access/refresh/expires_in
5. upsertToken(user_id, server_origin, tokens)：ON CONFLICT DO UPDATE，重置 refresh_fail_count=0 / refresh_fail_first_at=NULL
   - expires_at = now + expires_in（无 expires_in 则 +1800s）
6. notifyAgentReload(workspace_id, ['config'])  ← 跨服务通知 control_plane（见 workflow）
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
- [mcp_oauth_connect](../0003-mcp-oauth-connect/mcp_oauth_connect.md) — 作为 OAuth client 主驱动

### 关联术语
[MCP OAuth Client](../CONTEXT.md)、[MCP OAuth Token](../CONTEXT.md)、[Refresh Failure Grace](../CONTEXT.md)
