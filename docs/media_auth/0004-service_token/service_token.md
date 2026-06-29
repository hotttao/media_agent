---
name: service_token
description: B2B API token (tos_xxx) 的创建/列表/撤销，及 Platform Token 自动签发
dependencies:
    - auth
related_workflow:
    - service_token_verification
---

## 一句话职责
service_token 负责 B2B API 调用凭证 `tos_xxx` 的创建、列表、撤销，以及登录后自动 ensure 的 Platform Token；凭证以 SHA-256 hash 存储，明文仅创建时返回一次（Platform Token 例外，见 ADR-0002）。

## 边界

### 做什么
- 创建 Service Token：生成 `tos_`+32 随机字节，存 SHA-256 hash，明文仅本次响应返回。
- 列出当前用户有效 token（不含 hash/明文）。
- 撤销当前用户名下的非 platform token（`revoked_at` 软删）。
- 登录后 ensure Platform Token（每用户至多一个 active，自动签发、存明文）。
- 提供 `getPlatformToken(userID)` 供 control_plane 取 agent 回调用（内部接口）。
- 提供 `verifyByHash(hash)` 供 internal `token` service 的 VerifyToken 解析主体。

### 不做什么
- 不签发 session / OAuth token（归 [auth](../0001-auth/auth.md) / [oauth](../0005-oauth/oauth.md)）。
- 不在 HTTP 层暴露 Platform Token 创建/读取（Platform Token 仅登录自动 ensure + 内部接口）。
- 不允许经公开 API 撤销 Platform Token（DB `WHERE is_platform = false`）。
- 不做 token→主体的 HTTP 中间件鉴权（control_plane 中间件调 VerifyToken RPC，见 [token_verification](../../../workflow/0001-service-token-verification.md) workflow）。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 创建 token | 生成 tos_xxx，存 hash，明文仅返回一次 | `CreateServiceToken` `POST /api/service-tokens` |
| 列出 token | 当前用户有效 token 列表（无 secret） | `ListServiceTokens` `GET /api/service-tokens` |
| 撤销 token | 撤销本人非 platform token | `RevokeServiceToken` `DELETE /api/service-tokens/:id` |
| ensure Platform Token（内部） | 登录后保证每用户一个 active platform token | `ensurePlatformToken(userID)` |
| 取 Platform Token（内部） | 供 agent 回调取明文 | `getPlatformToken(userID)` |
| hash 查询（内部） | 供 VerifyToken 解析 | `getServiceTokenByHash(hash)` |

## 数据模型

### Tables — `service_tokens`

| 字段 | 描述 | 类型 | 关联表 | 关联说明 |
|:---|:---|:---|:---|:---|
| id | token id | text PK | — | — |
| name | token 名；platform 固定 `__platform__` | text | — | — |
| token_hash | SHA-256(token) | text | — | 鉴权查询主键 |
| token | 明文；仅 platform 非 NULL | text | — | 见 ADR-0002 |
| created_by | 创建者 | text | users.id | 鉴权主体 sub |
| created_at | 创建时间 | timestamptz | — | — |
| revoked_at | 撤销时间；非空=失效 | timestamptz | — | 软删 |
| is_platform | 是否 platform token | boolean | — | — |

约束：部分唯一索引 `idx_service_tokens_platform_user (created_by) WHERE is_platform=true AND revoked_at IS NULL`。

### UUID 生成规则
`id` 应用层短随机串；`token` = `"tos_" + hex(randomBytes(32))`；`token_hash` = SHA-256(token)。

## 能力详细说明

### CreateServiceToken
```
1. 入参：name（必填，缺 → 400）；sub = session.sub
2. token = "tos_" + hex(32 bytes)
3. INSERT { id, name, token_hash=sha256(token), created_by=sub, is_platform=false }
4. 返回 { id, name, token, created_at }  ← 明文仅此一次
```

### ListServiceTokens
```
1. SELECT id, name, created_by, created_at, revoked_at, is_platform WHERE created_by=sub
2. 返回列表（不含 token_hash / token）
```

### RevokeServiceToken
```
1. 入参：id（path）；sub = session.sub
2. UPDATE SET revoked_at=now() WHERE id=? AND created_by=sub AND is_platform=false
3. 0 行 → 404（不存在 / 非本人 / 是 platform / 已撤销）
```

### ensurePlatformToken（内部，登录后）
```
1. 入参：userID
2. 若已有 active platform token（is_platform=true AND revoked_at IS NULL）→ 直接返回
3. 否则 INSERT { name='__platform__', token=tos_xxx, token_hash, token=明文, created_by=userID, is_platform=true }
   - 唯一索引并发冲突 → 重读现有并返回
4. fire-and-forget，失败仅日志
```

### getServiceTokenByHash（内部，供 VerifyToken）
```
1. 入参：hash
2. SELECT * WHERE token_hash=? AND revoked_at IS NULL
3. 命中 → 返回记录（含 created_by、name、is_platform）
4. 未命中 → nil（→ VerifyToken 返回 invalid）
```

## 关联关系

### 依赖的 domain
- [auth](../0001-auth/auth.md) — 登录后触发 ensurePlatformToken

### 被依赖的 domain
- internal `token` service（VerifyToken）调用 `getServiceTokenByHash`

### 参与的 workflow
- [service_token_verification](../../../workflow/0001-service-token-verification.md) — 作为主体解析方参与跨服务校验

### 关联术语
[Service Token](../CONTEXT.md)、[Platform Token](../CONTEXT.md)、[VerifyToken](../CONTEXT.md)、[Subject](../CONTEXT.md)
