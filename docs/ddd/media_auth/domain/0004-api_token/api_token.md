---
name: api_token
description: User、Agent、Personal Worker 与受控客户端的长期 API 凭证
dependencies:
    - auth
related_workflow:
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
| API Token | Token Metadata、Actor Binding | 一把 Talos Key 的管理生命周期 | 一个 active Token 只绑定一个 Actor、一个被代表 User 和至多一个 Organization；Talos Key ID 全局唯一 |

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
| talos_key_id | text unique | Talos API Key 稳定标识，不是 Secret |
| subject_user_id | text nullable | 被代表业务 User；直接 Service Actor Token 可为空 |
| actor_type | text | user/agent/personal_worker/service |
| actor_id | text | `User:*`、`Agent:*`、`Worker:*` 或 `Service:*` |
| organization_id | text nullable | 唯一企业上下文 |
| name | text | 用户可读名称 |
| scopes | text[] | 入口能力上限 |
| status | text | active/revoked/expired |
| expires_at | timestamptz | 到期时间 |
| created_by | text | 创建命令的 User 或受信服务主体 |
| created_at / rotated_at / revoked_at | timestamptz nullable | 生命周期时间 |

### UUID 生成规则

本地管理 ID 使用 ULID；Secret 格式和 Talos Key ID 由 Talos 配置决定。

## 能力详细说明

### CreateAPIToken

1. 从 Web Session 或受信服务身份取得创建者，不接受客户端覆盖 User。
2. 校验 Actor 存在、active，并验证 `actor -> owner User -> Organization` 关系。
3. 校验 scope 不超过创建者可委托范围，过期时间在策略上限内。
4. 调 Talos 创建 `actor_id` 对应 API Key；成功后事务保存本地元数据。
5. 本地提交失败时立即撤销已创建的 Talos Key；撤销结果不明确进入补偿队列，不能再次展示 Secret。

### VerifyAPIToken

1. 仅允许 Edge、Agent Gateway 和受信服务调用；原始 Token 不记录。
2. 调 Talos verify，取得 `talos_key_id/actor_id/scopes/expiry` 并与本地元数据交叉校验。
3. 加载当前 User、Actor、Organization Membership 和 Token 状态，拒绝任何失效关系。
4. User 直接调用返回 `sub=User`；Agent/Personal Worker 返回 `sub=User + act.sub=Actor`；Service 返回 `sub=Service`。
5. 返回 Principal，由调用方签发精确 audience、1 至 5 分钟 TTL 的 Internal Identity JWT。

### Rotate/Revoke

轮换与撤销属于跨 media_auth/Talos Workflow：本地先记录 pending operation，以稳定 operation ID 调 Talos，成功后提交终态；超时先按 `talos_key_id` 查询，禁止盲目创建第二把 Key。Actor 或 User 删除时后台命令幂等撤销全部关联 Token。
