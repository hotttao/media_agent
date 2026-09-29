---
name: api_token
description: User、Agent、Personal Worker 与受控客户端的长期 API 凭证
dependencies:
    - auth
related_workflow:
    - api_token_issuance
    - api_token_rotation
    - api_token_revocation
    - api_token_verification
    - agent_api_token_provisioning
---

## 一句话职责

API Token 负责长期 opaque API 凭证的管理元数据、Actor 绑定、Organization/scope 限制和 Talos 生命周期协调；Talos 持有 Secret，media_auth 持有业务授权上下文。

## 边界

### 做什么

- 为 User、Agent、Personal Worker 或受控 Service 创建 Talos API Key，Secret 只返回一次。
- 保存 Token Manager ID、`talos_key_id`、被代表 User、当前 Actor、Organization、scope、状态与时间信息。
- 查询、轮换和撤销当前调用者有权管理的 API Token。
- 向 Edge 和 Agent Gateway 提供 `VerifyAPIToken`，返回签发 Internal Identity JWT 所需的 Principal。

### 不做什么

- 不自行生成另一套长期 Token，不保存可恢复 Secret 或 Secret Hash。
- 不处理 Web Session、OAuth Access Token、Session Token 或 Platform Token。
- 不拥有 Agent、Worker、Organization 或资源权限事实；验证时查询其权威来源。
- 不替业务服务执行最终资源授权。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 创建令牌 | 校验 Actor 委托后调用 Talos，Secret 仅本次返回 | `CreateAPIToken` |
| 列出令牌 | 返回调用者可管理的 Token 元数据，不返回 Secret | `ListAPITokens` |
| 轮换令牌 | 调 Talos 生成新 Secret 并撤销旧 Secret | `RotateAPIToken` |
| 撤销令牌 | 撤销 Talos Key 并提交本地状态 | `RevokeAPIToken` |
| 验证令牌 | Talos verify 后组合当前 User/Actor/Organization 事实 | `VerifyAPIToken` |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| API Token | Token Metadata、Actor Binding、Rotation Operation | 管理生命周期与本次轮换意图 | 一个 active Token 只绑定一个 Actor、一个被代表 User 和至多一个 Organization；Talos Key ID 全局唯一 |

- Secret 只在创建或轮换响应中出现，不能写入数据库、日志、Trace、Outbox 或审计载荷。
- User Token 的 `actor_ref` 等于其 User；Agent/Personal Worker Token 必须同时固定 `subject_user_id` 和不同的 `actor_ref`。
- Agent/Worker 不能用请求体覆盖 owner User；创建者必须通过受信 Gateway 编排或当前 User 对 Actor 的所有权校验。
- scope 是凭证能力上限，不代表资源授权；User、Actor、Organization 和资源授权在请求时重新检查。
- Actor disabled/deleted、User disabled/deleted、Membership 失效、Token revoked/expired 中任一成立即验证失败。
- Token 轮换产生新 Secret 并使旧 Secret失效；重复撤销幂等，不能恢复 revoked Token。
- Talos 是 Secret 状态事实源，本地状态用于管理、授权上下文与审计；不允许本地 Hash 验证旁路 Talos。

## 数据模型

### `api_tokens`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | 面向用户和 Gateway 的 Token Manager ID |
| talos_key_id | text nullable unique | Talos API Key 稳定标识，不是 Secret；issuing 时尚无映射 |
| subject_user_id | text nullable | 被代表业务 User；直接 Service Actor Token 可为空 |
| actor_type | text | user/agent/personal_worker/service |
| actor_id | text | `User:*`、`Agent:*`、`Worker:*` 或 `Service:*` |
| organization_id | text nullable | 唯一企业上下文 |
| name | text | 用户可读名称 |
| scopes | text[] | 入口能力上限 |
| status | text | issuing/issuance_cleanup/active/rotating/revoking/revoked/expired；只有 active 可验证 |
| issuance_deadline | timestamptz nullable | 签发尝试允许正常激活的期限 |
| client_request_key | text nullable | 创建者范围内的客户端去重键，不直接发给 Talos |
| expires_at | timestamptz | 到期时间 |
| created_by | text | 创建命令的 User 或受信服务主体 |
| created_at / rotated_at / revoked_at | timestamptz nullable | 生命周期时间 |

### 约束与索引

`talos_key_id` 唯一；`revoking` 状态使用按 ID 的部分索引供后台 keyset 扫描。

issuing/issuance_cleanup 保存聚合内的持久创建意图，并使用按 ID 的部分索引供恢复扫描；不存在 Key 时使用 NULL 而不是空字符串。`(created_by, client_request_key)` 对非空键和非 revoked/expired 状态保持唯一，重复命令不能重放 Secret。active 必须有非空 Key；清理状态不能重新激活。

- issuing 必须具有 issuance_deadline 且 Key 为 NULL；issuance_cleanup 必须具有期限。其余状态必须具有非空 Key。
- 创建完成同时 CAS issuing、NULL Key、期限和未来有效期；清理领取与激活互斥。清理完成只允许同一 Key 幂等重放，不能覆盖另一个终态 Key。
- 普通 MarkRevoked 只允许 revoking/revoked；未完成创建必须走 FinishIssuanceCleanup，不能用普通撤销丢弃意图。
- 客户端键为空时保存 NULL，非空最大200字符；数据库唯一冲突统一映射为领域 conflict，不暴露驱动细节。

### ID 生成规则

本地管理 ID 使用应用层 ID 生成器产生的 32 位随机十六进制串；Secret 格式和 Talos Key ID 由 Talos 配置决定。

### `api_token_rotations`

依附 API Token 的轮换尝试实体；正常完成和失败恢复都必须与 Token 状态原子提交。没有独立授权或生命周期，不保存 Secret。

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | 稳定轮换操作 ID |
| token_id | text FK | 所属 Token Manager ID |
| old_key_id | text | 发起时冻结的 Talos Key ID |
| actor_id | text | 发起时冻结的 Actor，用于上游列表过滤 |
| status | text | pending/recovering/completed/aborted |
| deadline | timestamptz | 同步提交期限与后台恢复条件 |
| created_at | timestamptz | 创建时间 |

#### 约束与索引

- 单个 Token 至多一条 pending/recovering 操作（部分唯一索引）；终态记录保留审计。
- pending/recovering 使用按 ID 的部分索引扫描；API Token 与操作的更新必须在同一事务提交。
- 已取得恢复的操作不能 completed；completed 不能恢复；终态不能重开。

## 能力详细说明

### CreateAPIToken

1. 从 Web Session 或受信服务身份取得创建者，不接受客户端覆盖 User。
2. 校验 Actor 存在、active，并验证 `actor -> owner User -> Organization` 关系。
3. 校验 scope 不超过创建者可委托范围，过期时间在策略上限内。
4. 先保存 issuing 意图和期限，成功激活必须同时满足 issuing、期限内以及完整 Key 映射；持久提交后才返回一次性 Secret。
5. 失败/取消/撤销进入 issuance_cleanup，清理状态禁止激活；所有本次 Key 确认撤销后才进入 revoked。外部副作用、幂等屏障与恢复见 [API Token Issuance](../../workflow/0010-api-token-issuance/api_token_issuance.md)。

### VerifyAPIToken

1. 仅允许 Edge、Agent Gateway 和受信服务调用；原始 Token 不记录。
2. 调 Talos verify，取得 `talos_key_id/actor_id/scopes/expiry` 并与本地元数据交叉校验。
3. 加载当前 User、Actor、Organization Membership 和 Token 状态，拒绝任何失效关系。
4. User 直接调用返回 `sub=User`；Agent/Personal Worker 返回 `sub=User + act.sub=Actor`；Service 返回 `sub=Service`。
5. 返回 Principal，由调用方签发精确 audience、1 至 5 分钟 TTL 的 Internal Identity JWT。

### Rotate/Revoke

撤销在外部调用前提交 `revoking`，立即禁止验证；以 Token Manager ID 和 Talos Key ID 为稳定操作标识，后台恢复同一撤销意图。具体事务、重试与结果确认见 [API Token Revocation](../../workflow/0008-api-token-revocation/api_token_revocation.md)。

轮换先提交 rotating 与操作记录，以操作 ID 标记 Talos replacement metadata；期限内正常提交后一次性返回 Secret。失败或超时按 [API Token Rotation](../../workflow/0009-api-token-rotation/api_token_rotation.md) 撤销旧 Key、枚举并清理本次 replacement，终态 revoked；不重发 Rotate，不持久化或重放 Secret。Actor 或 User 删除时后台命令幂等撤销全部关联 Token。
