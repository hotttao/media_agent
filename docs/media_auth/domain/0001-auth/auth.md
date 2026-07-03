---
name: auth
description: 本地账号 + LDAP 登录、会话(JWT cookie)签发与校验、当前用户档案与偏好
dependencies:
    - identity
related_workflow:
    - wecom_login_bind
---

## 一句话职责
auth 负责平台账号的本地密码 / LDAP 双路登录、会话（JWT cookie）的签发与校验、当前用户档案读取与本人偏好（default_prompt_id / auto_evolution）维护；Session 归本 domain。

## 边界

### 做什么
- 本地密码（argon2）与 LDAP 双路登录，LDAP 首次登录自动 upsert 用户。
- 签发 / 清除 / 半程滑动续期 httpOnly cookie `token`（HS256 JWT，24h）。
- 读取当前用户档案 `GetMe`（id/username/role/auth_source/default_prompt_id/auto_evolution）。
- 本地账号改密（仅 `password_hash` 非空）。
- 设置 / 清除 `default_prompt_id`，开关 `auto_evolution`。
- 登录成功后 ensure Platform Token（委托 `service_token` domain 的内部能力）。

### 不做什么
- 不签发 OAuth Access/Refresh Token（归 [oauth](../0005-oauth/oauth.md)）。
- 不管理外部身份绑定（归 [identity](../0002-identity/identity.md)）；仅按只读查询推导 `auth_source`。
- 不验证 Service Token（归 internal `token` service + [service_token](../0004-service_token/service_token.md)）。
- 不承载企微登录通道（归 [wecom](../0003-wecom/wecom.md)），但为其提供 Session 签发能力。
- 不解析 `default_prompt_name`（prompts 表留在 control_plane，name 由网关补，见 ADR-0001）。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 登录 | 本地密码 / LDAP 双路认证，签发 session cookie | `Login` `POST /api/auth/login` |
| 登出 | 清除 session cookie | `Logout` `POST /api/auth/logout` |
| 当前用户档案 | 返回当前用户完整档案（不含 prompt name） | `GetMe` `GET /api/auth/me` |
| 更新本人设置 | 局部更新 auto_evolution（tri-state） | `UpdateMe` `PATCH /api/auth/me` |
| 修改密码 | 本地账号改密（校验旧密码 + 长度≥6 + 异于旧密码） | `ChangePassword` `PUT /api/auth/me/password` |
| 设置默认 prompt | 设/清新 workspace 默认 prompt（opaque id） | `SetDefaultPrompt` `PUT /api/auth/me/default-prompt` |
| 签发 Session | 内部：登录/企微登录成功后签 JWT 并下 cookie | `issueSession(userID)` |
| 校验 Session | 内部：middleware 本地验 cookie JWT + 滑动续期 | `verifySession(cookie)` |

## 数据模型

### Tables — `users`

| 字段 | 描述 | 类型 | 关联表 | 关联说明 |
|:---|:---|:---|:---|:---|
| id | 用户 id，应用层生成短随机串 | text PK | — | 全局唯一 |
| username | 登录名，LDAP 派生账号用 LDAP attr | text UNIQUE | — | 登录主键 |
| display_name | 展示名 | text | — | JWT `name` |
| email | 邮箱，可空 | text | — | JWT `email` |
| password_hash | argon2 哈希；空=LDAP/企微派生，不可改密 | text | — | 决定 auth_source |
| role | `user \| admin \| system`；system 禁止登录 | text | — | JWT `role` |
| default_prompt_id | 默认 prompt；opaque，不校验 FK | text | control_plane.prompts | 网关补 name |
| auto_evolution | 偏好开关 | boolean | — | — |
| created_at | 创建时间 | timestamptz | — | — |
| last_login_at | 最近本地登录时间 | timestamptz | — | 仅本地登录更新 |

### UUID 生成规则
`users.id` 由应用层 `generateId()` 产生短随机字符串，非自增、非 ulid；与 control_plane 现状一致以兼容存量数据。

## 能力详细说明

### Login
```
1. 入参：username, password
2. authenticateLocal
   - 按 username 查 users；若 password_hash 非空，argon2.Verify(password, hash)
   - 命中 → user
3. 若本地失败 → authenticateLdap
   - LDAP bind + search（LDAP_URL / BIND_DN / SEARCH_BASE / SEARCH_FILTER）
   - 命中 → upsertUser(ldap attrs)：已存在更新 last_login_at，不存在则创建（password_hash 留空）
4. 两者皆失败 → 401 invalid_credentials
5. role == 'system' → 403（"This account cannot be used for login"）
6. issueSession(user)：
   - JWT claims = { sub, username, name, email?, role, exp=now+86400 }，HS256 签
   - Set-Cookie token（httpOnly, sameSite=Lax, maxAge=86400, path=/）
7. fire-and-forget ensurePlatformToken(user.id)（失败仅日志）
8. 返回 ApiUser{ id, username, role, auth_source, default_prompt_id, auto_evolution }
```

### auth_source 推导（修正点，见 change.md）
```
password_hash 非空            → 'password'
否则且 user 有 wecom identity  → 'wecom'   ← 修正：control_plane 误报 'ldap'
否则                          → 'ldap'
```
> control_plane 现状对企微派生账号返回 `auth_source='ldap'`（因仅判 password_hash），proto 注释已声明应为 `wecom`。media_auth 修正为读 identity 推导，需 auth domain 只读 identity 查询。

### ChangePassword
```
1. 入参：current_password, new_password（仅本人，sub 来自 session）
2. user 无 password_hash → 400（LDAP/企微账号不可改密）
3. argon2.Verify(current_password, hash) 失败 → 401
4. len(new_password) < 6 → 400；new_password == current_password → 400
5. 更新 password_hash = argon2.Hash(new_password)
```

### GetMe
```
1. sub = session.sub；查 users
2. 推导 auth_source（见上，只读 identity）
3. 返回 ApiUser（default_prompt_id 仅 id，不含 name）
4. user 不存在 → 500（session 有效却无用户，数据异常）
```

### SetDefaultPrompt
```
1. 入参：prompt_id（optional；null=清除，非空=设置）
2. 写 users.default_prompt_id（opaque，不校验存在性）
```
> UpdateMe(auto_evolution) 与 SetDefaultPrompt 均用 proto3 `optional` 表达 tri-state / 显式 false。

### issueSession / verifySession（内部）
```
issueSession(user): 签 JWT + Set-Cookie（见 Login §6）
verifySession(cookie):
  - 解析验签 HS256；过期/非法 → 清 cookie，返回 invalid
  - 剩余 < 12h → 重签 24h 并替换 cookie（滑动续期）
  - 返回 JwtPayload{ sub, username, name, email?, role, exp }
```

## 关联关系

### 依赖的 domain
- [identity](../0002-identity/identity.md) — 只读查询，推导 `auth_source`
- [service_token](../0004-service_token/service_token.md) — 登录后委托 `ensurePlatformToken`

### 被依赖的 domain
- [wecom](../0003-wecom/wecom.md) — 企微登录成功后调 `issueSession` 签 cookie
- [oauth](../0005-oauth/oauth.md) — access/refresh token 的 user 解析复用 users 读取

### 参与的 workflow
- [wecom_login_bind](../../workflow/0001-wecom-login-bind/wecom_login_bind.md) — 在 workflow 中作为 session 签发方参与

### 关联术语
[Session](../../CONTEXT.md)、[User](../../CONTEXT.md)、[Auth Source](../../CONTEXT.md)、[Platform Token](../../CONTEXT.md)
