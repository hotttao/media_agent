---
name: service_token
description: 用户创建的 CLI/API 长期访问凭证
dependencies:
    - auth
related_workflow:
    - service_token_verification
---

## 一句话职责

Service Token 负责用户 CLI/API 凭证的创建、查询、撤销和内部内省。

## 边界

### 做什么
- 创建 `tos_` opaque Token，数据库只保存 SHA-256 Hash。
- 列出和撤销当前用户的 Token。
- 向 Edge 提供 `IntrospectServiceToken`。

### 不做什么
- 不处理 Web Session、OAuth Token、Session Token 或 Platform Token。
- 不替资源服务执行 Workspace 或资源授权。

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

1. 校验名称、scope 和可选有效期。
2. 生成随机 Token，只保存 Hash。
3. 明文仅在创建响应返回一次，日志与 Trace 必须脱敏。

### IntrospectServiceToken

1. 仅允许 Edge 和明确内部服务通过 mTLS 调用。
2. Hash 查询未撤销且未过期记录，并加载有效用户。
3. 返回 Principal；资源服务继续执行本地授权。
