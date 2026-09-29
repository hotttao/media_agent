---
name: worker_identity
description: Personal Worker 的稳定委托身份、归属和访问状态
dependencies:
  - auth
  - organization
related_workflow:
  - worker_access_revocation
  - api_token_verification
  - worker_token_provisioning
---

## 一句话职责

持有 Personal Worker Identity 的身份、不可变 Owner/Organization 和访问状态，不持有 Secret 或运行任务。

## 边界

### 做什么

- 为当前 active User 在其所属 active Organization 创建稳定 Worker Actor。
- 列出、读取本人身份；校验精确 `Worker:<id>`、owner、Organization 和状态。
- 停用/软删除身份，保留历史引用。

### 不做什么

- API Token 签发、Secret 生命周期归 api_token/Talos。
- Capability 注册、Dispatch、lease、任务领取和结果归 media_workflow。
- 进程实例、GPU、Provider 凭据和本地执行策略归 media_worker。
- Agent 配置/业务生命周期归 media_control；Managed Worker 不注册 Personal Identity。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 创建身份 | 当前 User 固定 owner 与 Organization | Service.Create；CreateWorkerIdentity |
| 查询本人身份 | 不返回他人记录或 Token Secret | Service.List/GetOwned；ListWorkerIdentities/GetWorkerIdentity |
| 校验委托 | 精确 Actor、active、owner、Organization | Service.ValidateActor |
| 改变状态 | active→disabled；active/disabled→deleted | Service.ChangeStatus，仅由撤销 Workflow 调用；DisableWorkerIdentity/DeleteWorkerIdentity |
| 申请凭据 | Owner为已有active身份申请受限Token | CreateWorkerApiToken；WorkerTokenProvisioning |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| Worker Identity | 稳定 Actor 与绑定、状态时间 | 单次身份创建/状态 CAS | ID 不复用；owner/Organization 不变；非 active 不允许委托 |

当前版本不提供重新激活；deleted 保留 ID，不能物理删除或由新 Owner 接管。停用身份与关联 Token 的写入由 WorkerAccessRevocation 协调，不把 Token 放进 Identity 聚合。纯创建/查询无外部副作用，不创建额外 Workflow。

## 领域事件

无；撤销协调采用明确同库事务与持久 Token 状态。

## 数据模型

### `worker_identities`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | 服务生成的32位随机 ID；Actor 为 Worker:<id> |
| owner_user_id | text FK | 唯一被代表 User |
| organization_id | text FK | 唯一企业，不允许迁移 |
| name | text | 展示名称，不作为身份键 |
| status | text | active/disabled/deleted |
| created_at | timestamptz | 创建时间 |
| disabled_at | timestamptz nullable | 停用时间 |
| deleted_at | timestamptz nullable | 软删除时间 |

#### 约束与索引

- owner、Organization 为本服务事实，使用本库外键，不级联物理删除。
- disabled/deleted 必须有对应时间；名称非空、最多200字符。
- `(owner_user_id,created_at,id)` 查询本人列表；`(id) WHERE status<>'active'` 支持后续撤销恢复扫描。
- down migration 遇到任何 Identity 记录拒绝，不静默删除身份或历史审计绑定。

## 能力详细说明

Create 从可信 User 上下文取得 owner，只在当前 User active 且是 active Organization 成员时创建；缺失校验器或上游不可用拒绝。名称 trim，默认 active，客户端不能指定 ID/状态/owner。

GetOwned 对他人返回 not-found。ValidateActor 严格接受 Worker 前缀与非空 ID，逐次查询身份；仅 active 且 owner/Organization 都精确匹配时成功，不缓存成功结果，不接受 Agent/Service/实例 ID。

ChangeStatus 校验 owner 和允许的转换，并以旧状态 CAS；同状态幂等，deleted 不可退出。调用者必须通过撤销 Workflow 同时保存 Token 的清理意图，不把单独状态更新作为完整停用操作。

## 关联关系

- [api_token](../0004-api_token/api_token.md) 通过稳定回调读取身份事实，验证并不取得身份写权限。
- [Worker Access Revocation](../../workflow/0011-worker-access-revocation/worker_access_revocation.md) 协调停用与凭据清理。
- 术语：Personal Worker Identity、Delegated Actor、Worker Principal、API Token。
