---
name: identity
description: 用户外部身份绑定的 generic 查询与解绑，按 provider 路由
dependencies: []
related_workflow:
    - wecom_login_bind
---

## 一句话职责
identity 负责用户外部身份绑定（`(provider, external_id) → user_id`）的列表与解绑，按 provider 路由，是企微等外部身份通道与账号之间的映射层。

## 边界

### 做什么
- 列出当前用户的全部外部身份绑定（provider/external_id/display_name/created_at）。
- 按 provider 解绑当前用户的某条身份绑定。
- 维护绑定唯一性：`(provider, external_id)` 全局唯一、`(user_id, provider)` 每用户每 provider 至多一条。

### 不做什么
- 不创建绑定（绑定由 provider 通道在 callback 中创建，归 [wecom](../0003-wecom/wecom.md)）。
- 不签发 session、不查 users 档案（归 [auth](../0001-auth/auth.md)）。
- 不与具体 provider 的 OAuth 流程耦合（identity 是 generic 的；企微流程归 wecom）。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 列出身份绑定 | 返回当前用户全部已绑 provider | `ListIdentities` `GET /api/auth/identities` |
| 解绑身份 | 按 provider 解绑当前用户的身份 | `DeleteIdentity` `DELETE /api/auth/identities/:provider` |
| 查找绑定（内部） | 按 (provider, external_id) 解析 user_id，供登录通道用 | `findUserIdentity(provider, externalID)` |
| 创建绑定（内部） | 写 (user_id, provider, external_id, display_name, metadata)，供 bind 通道用 | `bindIdentity(...)` |

## 数据模型

### Tables — `user_identities`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| user_id | text | 所属用户；关联 `users.id (CASCADE)`（复合 PK 一段） |
| provider | text | 身份提供方标识（如 wecom） |
| external_id | text | provider 侧用户 id |
| display_name | text | provider 侧展示名 |
| metadata | jsonb | provider-specific 字段（avatar/email 等） |
| created_at | timestamptz | 创建时间 |

约束：PK `(provider, external_id)`；UNIQUE `(user_id, provider)`；INDEX `(user_id)`。

### UUID 生成规则
无独立 id；主键即 `(provider, external_id)`。

## 能力详细说明

### ListIdentities
```
1. sub = session.sub
2. SELECT provider, external_id, display_name, created_at WHERE user_id = sub
3. 返回列表（不含 metadata 内部细节，仅对外字段）
```

### DeleteIdentity
```
1. 入参：provider（path）
2. DELETE WHERE user_id = sub AND provider = ?
3. 0 行影响 → 404（无该 provider 绑定）
4. 否则 success
```
> control_plane 原路径为 `DELETE /api/auth/wecom/identity`（无 provider 参数、写死 wecom）。media_auth 泛化为 `/api/auth/identities/:provider`，是 P2 前端需适配的破坏性变更（见 change.md）。

### findUserIdentity（内部，供 wecom login）
```
1. 入参：provider, external_id
2. SELECT user_id WHERE provider=? AND external_id=?
3. 无 → 返回 not_bound（由 wecom domain 决定 redirect）
4. 有 → 返回 user_id
```

### bindIdentity（内部，供 wecom bind）
```
1. 入参：user_id, provider, external_id, display_name, metadata
2. 先查 (provider, external_id) 是否已绑给别的 user → 已绑 → already_bound
3. INSERT ON CONFLICT (user_id, provider) DO UPDATE（每用户每 provider 至多一条）
4. 返回 success
```

## 关联关系

### 被依赖的 domain
- [auth](../0001-auth/auth.md) — 只读查询推导 auth_source
- [wecom](../0003-wecom/wecom.md) — 登录解析 user_id、绑定写入

### 参与的 workflow
- [wecom_login_bind](../../workflow/0001-wecom-login-bind/wecom_login_bind.md) — 作为身份映射层参与

### 关联术语
[Identity](../../CONTEXT.md)、[Provider](../../CONTEXT.md)、[WeCom Login Channel](../../CONTEXT.md)
