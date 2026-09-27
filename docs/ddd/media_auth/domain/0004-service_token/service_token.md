---
name: service_token
description: 用户创建的 CLI/API 长期访问凭证
dependencies:
    - auth
related_workflow:
    - service_token_verification
---

## 一句话职责

Service Token 负责用户 CLI/API/Media Worker 凭证的创建、Organization 绑定、scope 限制、查询、撤销和内部内省。

## 边界

### 做什么
- 创建 `tos_` opaque Token，数据库只保存 SHA-256 Hash。
- 列出和撤销当前用户的 Token。
- 向 Edge 提供 `IntrospectServiceToken`，返回签发 Internal Identity JWT 所需的稳定 Principal。

### 不做什么
- 不处理 Web Session、OAuth Token、Session Token 或 Platform Token。
- 不替资源服务执行 Workspace 或资源授权。
- 不把 Worker Instance 建模为新的授权主体，也不拥有 Worker Dispatch 或 Conductor Task。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 创建令牌 | 生成凭证并仅本次返回明文 | `ServiceTokenService.Create` |
| 列出令牌 | 返回当前用户 Token 元数据 | `List` |
| 撤销令牌 | 使指定 Token 立即失效 | `Revoke` |
| 内省令牌 | 把 opaque Token 解析为 Principal | `AuthInternalService.IntrospectServiceToken` |

## 数据模型

### `service_tokens`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Service Token ID |
| user_id | text | 所属用户 ID |
| organization_id | text nullable | 可选企业绑定；Media Worker 用 Token 必填 |
| name | text | 用户可读名称 |
| token_hash | bytea | Token 的 SHA-256 Hash，全局唯一 |
| scopes | text[] | 粗粒度调用范围，不替代资源 ACL |
| expires_at | timestamptz nullable | 可选过期时间 |
| revoked_at | timestamptz nullable | 撤销时间，非空即失效 |
| created_at | timestamptz | 创建时间 |

### UUID 生成规则

ID 使用 ULID；Token 使用 `tos_` + 至少 256 位随机数。

## 能力详细说明

### Create

1. 校验名称、scope、可选 Organization 绑定和有效期；创建 Worker 用 Token 时 Organization 与有效期必填。
2. 校验创建者当前属于目标 Organization，且申请 scope 不超过其可委托范围。
3. 生成随机 Token，只保存 Hash。
4. 明文仅在创建响应返回一次，日志与 Trace 必须脱敏。

### IntrospectServiceToken

1. 仅允许 Edge 和明确内部服务通过 mTLS 调用。
2. Hash 查询未撤销且未过期记录，并加载有效用户与当前 Organization Membership。
3. 对 Worker route 要求唯一 Organization 绑定，并返回 `Principal{user_id, organization_id, credential_id, scopes, auth_source=service_token}`；普通 CLI Token 可不带 Organization。
4. Edge 以该 Principal 签发目标 audience 的短时 Internal Identity JWT；资源服务继续执行本地授权。

## 聚合不变量

- Token 明文不可恢复，Hash 全局唯一；撤销不可逆。
- scope 是入口能力上限，不代表资源授权；Membership、Entitlement 与资源 owner 必须在请求时重新判断。
- Worker 用 Token 必须绑定一个 Organization，不允许客户端在每次请求中切换 Organization。
- `credential_id` 标识本 Token；业务授权主体始终是 `user_id`，不能把 Credential 或 Worker Instance 当成 User。
- 原始 Token 不进入 Internal JWT、业务服务、日志、Trace、Workflow Definition 或 Conductor payload。
