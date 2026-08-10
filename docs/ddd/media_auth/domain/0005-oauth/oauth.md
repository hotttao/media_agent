---
name: oauth
description: media_auth 作为 OAuth2 Authorization Server，authorization_code/refresh_token/token-exchange 三 grant + userinfo
dependencies:
    - auth
related_workflow:
    - oauth_authorize_code_flow
---

## 一句话职责
oauth 负责媒体平台作为 OAuth2 Authorization Server 的全部能力：第三方 client 注册校验、authorization_code（含 PKCE S256）签发与消费、refresh_token 轮转、RFC 8693 token-exchange、userinfo 解析。

## 边界

### 做什么
- 校验 OAuth Client（redirect_uri 精确匹配白名单、client_secret 常量时间比较）。
- 签发一次性 authorization_code（10min TTL，消费即删，可带 PKCE S256）。
- `ExchangeToken` 三 grant 合一：authorization_code / refresh_token / token-exchange。
- refresh_token 轮转（旧删新发，30d TTL，hash 存储）。
- token-exchange：校验外部 RS `/introspect` 后签发 access_token。
- 签发 OAuth Access Token（HS256 JWT，1h TTL，同 JWT_SECRET）。
- userinfo：Bearer access_token 解析用户信息。
- 定时清理过期 authorization_code / refresh_token。

### 不做什么
- 不签发 session cookie（归 [auth](../0001-auth/auth.md)）；access_token 是 JWT 不是 cookie。
- 不做 media_auth 作为 OAuth client 连第三方（角色相反，归 [mcp_oauth](../0006-mcp_oauth/mcp_oauth.md)）。
- 不校验 scope 取值集合（scope 透传，默认 `profile`，不做 scope 集 validation）。
- 不维护 client 注册 HTTP 接口（client 注册由控制面/脚本维护，本批次 IDL 不暴露 client CRUD）。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 授权信息 | 返回 client 元信息供 consent 页 | `GetAuthorizeInfo` `GET /api/oauth/authorize` |
| 批准授权 | 签发 authorization_code，拼好 redirect URL | `ApproveAuthorization` `POST /api/oauth/authorize` |
| 换 token | 三 grant 合一入口（form-urlencoded） | `ExchangeToken` `POST /api/oauth/token` |
| 用户信息 | Bearer access_token 解析用户 | `GetUserInfo` `GET /api/oauth/userinfo` |

## 数据模型

### Tables — `oauth_clients`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | client id |
| name | text | client 名（consent 展示） |
| secret_hash | text | SHA-256(client_secret)；空=公开 client |
| redirect_uris | text[] | 允许回调，精确匹配 |
| homepage_url / description | text | consent 展示 |
| created_by | text | 创建者；关联 `users.id` |
| created_at / updated_at | timestamptz | 时间 |

### Tables — `oauth_authorization_codes`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| code | text PK | 一次性码，32 字节 hex |
| client_id | text | 所属 client；关联 `oauth_clients (CASCADE)` |
| user_id | text | 授权用户；关联 `users (CASCADE)` |
| redirect_uri | text | 回调 |
| code_challenge | text | PKCE S256 challenge |
| scope | text | 默认 profile |
| expires_at | timestamptz | 10min TTL |

### Tables — `oauth_refresh_tokens`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| token_hash | text PK | SHA-256(refresh_token) |
| client_id | text | 所属 client；关联 `oauth_clients (CASCADE)` |
| user_id | text | 所属用户；关联 `users (CASCADE)` |
| expires_at | timestamptz | 30d TTL |
| created_at | timestamptz | 创建时间 |

> Access Token 不落库：HS256 JWT，自包含。

### UUID 生成规则
`code` / `refresh_token` = hex(randomBytes(32))；`access_token` = HS256 JWT（claims: sub/username/name/email/role/exp）；`client_secret` hash = SHA-256。

## 能力详细说明

### GetAuthorizeInfo
```
1. 入参：client_id, redirect_uri, response_type, scope
2. response_type != 'code' → 400 unsupported_response_type
3. client 不存在 → 400 invalid_client
4. redirect_uri 不在白名单 → 400 invalid_redirect_uri
5. 返回 { client_name, scope }
```

### ApproveAuthorization
```
1. 入参：client_id, redirect_uri, response_type, state?, code_challenge?, code_challenge_method?, scope
2. 同上校验 + code_challenge_method 仅支持 S256（否则 400）
3. code = hex(32)；INSERT authorization_code { code, client_id, user_id=session.sub, redirect_uri, code_challenge, scope, expires_at=now+10min }
4. redirect_uri = {redirect_uri}?code={code}&state={state}
5. 返回 { redirect_uri }
```

### ExchangeToken（按 grant_type 分发，form-urlencoded）
```
grant_type=authorization_code:
  - 取 code（DELETE ... RETURNING，一次性）；无/过期 → 400 invalid_grant
  - client_id 不匹配 / redirect_uri 不匹配 → 400
  - code_challenge 存在但缺 code_verifier → 400；sha256(verifier) base64url != challenge → 400 invalid_grant
  - access = signJWT(user, 1h)；refresh = hex(32)，存 hash（30d）
  - 返回 { access_token, refresh_token, token_type=Bearer, expires_in=3600 }

grant_type=refresh_token:
  - hash(refresh_token) 查表；无/过期 → 400；client_id 不匹配 → 400
  - 旧 refresh DELETE（轮转）；签新 access + 新 refresh
  - 返回 { access_token, refresh_token, expires_in=3600 }

grant_type=urn:ietf:params:oauth:grant-type:token-exchange:
  - subject_token_type 须为 access_token 类型或缺省；缺 subject_token / resource → 400
  - confidential client：校验 client_secret（常量时间）
  - POST {resource.origin}/introspect (token=subject_token)；不 active → 400 invalid_grant
  - 解析 subject → user 不存在 → 500
  - access = signJWT(user, 1h)；不发 refresh
  - 返回 { access_token, issued_token_type=urn:...:access_token, expires_in=3600 }

其它 grant_type → 400 unsupported_grant_type
```
> `/api/oauth/token` 是本设计唯一 form-urlencoded 端点（RFC 6749 §3.2）。control_plane 现状兼容 form+JSON，media_auth IDL 强制 form，JSON fallback 由 handler 手动解析、不在 IDL 表达。

### GetUserInfo
```
1. Authorization: Bearer <access_token>；本地 HS256 验签
2. 无效/过期 → 401
3. 返回 { sub, username, name, email? }
```

### 清理（定时）
每小时 DELETE 过期 authorization_code 与 refresh_token（control_plane 现状 setInterval，media_auth 沿用定时任务）。

## 关联关系

### 依赖的 domain
- [auth](../0001-auth/auth.md) — 复用 users 读取 + JWT 签名密钥/算法

### 参与的 workflow
- [oauth_authorize_code_flow](../../workflow/0002-oauth-authorize-code-flow/oauth_authorize_code_flow.md) — 作为 AS 主驱动

### 关联术语
[OAuth Client](../../CONTEXT.md)、[Authorization Code](../../CONTEXT.md)、[Refresh Token](../../CONTEXT.md)、[Access Token](../../CONTEXT.md)、[Token Exchange](../../CONTEXT.md)
