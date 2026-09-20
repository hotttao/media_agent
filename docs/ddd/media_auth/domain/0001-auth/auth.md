---
name: auth
description: 用户账号生命周期、登录、Cookie JWT 与当前用户档案
dependencies:
    - identity
related_workflow:
    - wecom_login_bind
    - user_soft_delete
    - user_suspension
---

## 一句话职责
auth 负责平台账号的本地密码 / LDAP 双路登录、会话（JWT cookie）的签发与校验、当前用户档案读取与本人偏好（default_prompt_id / auto_evolution）维护；Session 归本 domain。

## 边界

### 做什么
- 本地密码（argon2）与 LDAP 双路登录，LDAP 首次登录自动 upsert 用户。
- 使用 media_auth 私钥签发、清除和续期 httpOnly Cookie JWT，并通过 JWKS 发布验证公钥。
- 读取当前用户档案 `GetMe`（id/username/role/auth_source/default_prompt_id/auto_evolution）。
- 本地账号改密（仅 `password_hash` 非空）。
- 设置 / 清除 `default_prompt_id`，开关 `auto_evolution`。
- 登录成功后可 ensure Platform Token（委托 `platform_token` domain 的内部能力）。
- 管理 User 的临时禁用与恢复；禁用会撤销现有会话和长期凭证，恢复不复活旧凭证。
- 软删除 User：禁止新登录，撤销全部凭证并保留历史 user_id。

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
| 禁用/恢复用户 | 管理员在 active 与 disabled 间切换；禁用时撤销访问能力 | `PATCH /api/users/{id}/status` |
| 软删除用户 | 管理员将用户标记 deleted 并撤销全部访问能力 | `DELETE /api/users/{id}` |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| User | 用户档案、登录状态 | 单个用户生命周期 | deleted 不可恢复为 active；历史 user_id 永不复用 |

- User 状态为 `active/disabled/deleted`；disabled 可由管理员恢复，deleted 为终态。
- disabled/deleted User 不能登录、续期 Cookie JWT、创建新 Service/Platform Token 或发起新的代表调用。
- 软删除不级联删除历史 Session、Task、Workspace、Team 审计记录；跨服务保留稳定 user_id。
- 禁用、恢复和删除事务都递增 `auth_version`；禁用/删除发布 `UserAccessRevoked`。恢复只允许签发新凭证，不恢复已经撤销的凭证或 Kratos Session。
- 浏览器身份只能从已验证的 Kratos Session 建立业务用户映射；`metadata_admin.role=platform_admin` 是受信任的管理员引导声明，只能把对应业务用户提升为平台管理员，客户端请求体不能直接声明该权限。

## 数据模型

### `users`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | 稳定 User ID，删除后不复用 |
| username | text UNIQUE | 登录名，删除后不释放 |
| display_name | text | 展示名 |
| email | text nullable | 邮箱 |
| password_hash | text nullable | Argon2 Hash；空表示外部身份账号 |
| role | text | user/admin/system；system 禁止交互登录 |
| status | text | active/disabled/deleted |
| auth_version | bigint | 凭证撤销版本，初始为 1 |
| default_prompt_id | text nullable | Control Prompt 的逻辑引用 |
| auto_evolution | boolean | 用户偏好开关 |
| disabled_at | timestamptz nullable | 禁用时间 |
| deleted_at | timestamptz nullable | 软删除时间 |
| deleted_by | text nullable | 执行删除的管理员 User ID |
| created_at | timestamptz | 创建时间 |
| last_login_at | timestamptz nullable | 最近登录时间 |

#### 约束与索引

- `CHECK (status IN ('active','disabled','deleted'))`
- `status='deleted'` 时 `deleted_at` 非空。
- username 和 id 不因软删除释放或复用。

## 领域事件

| 事件 | 产生条件 | 关键载荷 | 消费方 |
|:---|:---|:---|:---|
| UserDisabled | active -> disabled | user_id, auth_version | Edge、Token 撤销 Worker |
| UserSoftDeleted | active/disabled -> deleted | user_id, deleted_by, auth_version | 审计、资源所有服务 |
| UserAccessRevoked | disabled/deleted 提交 | user_id, auth_version | Edge 撤销缓存、Auth Token Domains |

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
5. user.status 必须为 active；disabled/deleted 一律拒绝且不签发 Cookie。
6. role == 'system' → 403（"This account cannot be used for login"）
7. issueSession(user)：
   - JWT claims = { sub, username, name, role, auth_version, type=cookie, aud=edge, exp }，由 media_auth 私钥签名
   - Set-Cookie token（httpOnly, sameSite=Lax, maxAge=86400, path=/）
8. fire-and-forget ensurePlatformToken(user.id)（失败仅日志）
9. 返回 ApiUser{ id, username, role, auth_source, default_prompt_id, auto_evolution }
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
issueSession(user): 仅 active User 可签 JWT + Set-Cookie（见 Login §6）
verifySession(cookie):
  - media_auth middleware 校验签名、issuer/audience/type/exp，并读取 User status/auth_version
  - 旧 control-plane 切换期调用内部 Session introspection，旧版本立即拒绝
  - 纯 JWKS 本地验签只证明 token 完整性；在 UserAccessRevoked 消费者完成前不提供即时撤销
  - 续期必须回到 Auth 并重新检查 User status/auth_version
```

### SoftDeleteUser

1. 要求管理员身份，禁止删除最后一个有效平台管理员。
2. 在 User 本地事务中将 status 置为 deleted、写 deleted_at/deleted_by、递增 auth_version，并写 Outbox。
3. Token Worker 幂等撤销该用户全部 Service Token、Platform Token 和 MCP OAuth Credential。
4. 当前旧 control-plane 通过 Session introspection 立即拒绝旧 Cookie JWT；未来 Edge 消费 `UserAccessRevoked` 后，才可按 user_id 维护最低 auth_version 缓存。
5. 已开始请求允许自然结束；删除提交后禁止新请求和后台长期委托启动。

### UpdateUserStatus

1. 仅接受 `active | disabled`；`deleted` 只能由 SoftDeleteUser 进入且不可恢复。
2. `active -> disabled` 禁止操作者禁用自己，并保护最后一个 active 平台管理员；事务写 `disabled_at`、递增 `auth_version` 并写撤销 Outbox。
3. 撤销 Worker 幂等撤销 Service/Platform/OAuth/MCP 凭证，并通过 Kratos Admin API 删除该 Identity 的全部 Web Session。
4. `disabled -> active` 清空 `disabled_at` 并再次递增 `auth_version`；旧凭证和旧 Session 不复活，用户必须重新登录或重新创建委托凭证。
5. 请求把 User 设置为其当前状态视为幂等成功；任何从 `deleted` 出发的状态修改返回冲突。

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
