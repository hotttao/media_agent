---
name: agent_instance
description: Gateway 实例注册、通信身份与业务排空准入
dependencies: []
related_workflow: [agent_workload_provisioning, session_replica_routing, agent_instance_registration]
---

## 一句话职责

Agent Instance 拥有副本当前运行代次与受控通信注册；Kubernetes 原生健康和 Control 期望配置不是本 Domain 的事实。

## 边界

### 做什么

拥有已验证的实例通信注册、运行代次、业务准入与排空命令幂等事实。

### 不做什么

Control 拥有配置与环境授权；Kubernetes 拥有原生资源健康；Agent 拥有 Session/Turn 执行事实。身份核验、资源查询与迁移由 Workflow 协调。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 注册代次 | 接收 Workflow 已核验身份，CAS 当前登记并保存历史 | `Service.Register` |
| 查询实例 | 返回当前/历史代次及准入事实 | `Service.Get/List/Historical` |
| 标记排空 | operation_id/expected incarnation 幂等；保留持久准入屏障 | `Service.Drain` |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| Instance Registration | 当前 incarnation、受控通信地址、admission | 单次注册或排空 CAS | 旧代次不覆盖新代次；同代次重注册不能解除 draining |

Instance Registration 根为 environment/workspace/replica；当前 incarnation 与业务 admission 由本地 CAS 更新。Instance Actor 不替代稳定 User/Agent 委托身份，不把自报 endpoint 或副本数当作事实。Gateway 读取 Kubernetes 资源身份，Docker 使用创建记录；Controller 不登记/探活 Agent。

## 领域事件

无独立消息事件。Workflow 查询已提交注册/准入事实进行协调，提交后通知失败不回滚注册。

## 数据模型

### agent_instances

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| environment_id, workspace_id, replica_id | text composite PK | 稳定实例定位 |
| agent_config_id | text | Control 的稳定 Agent 配置 ID |
| incarnation_id | text | 已验证本次启动代次 |
| credential_hash | text | 已验证当前代次的独立凭证摘要；稳定 Agent 身份不能替代实例证明 |
| resource_generation | text | 容器 ID 与启动时间组成的原生运行代次；同 generation 不接受不同 incarnation |
| agent_type | text | 原生资源标识的 Runtime 类型，与 Session 固定类型匹配 |
| resource_name / namespace | text | 原生资源定位，供后续核验及退出证据查询 |
| credential_key_id | text | 独立实例凭证 MAC 的持久密钥版本，丢响应可重建 |
| resource_uid | text | Kubernetes/Docker 创建资源引用 |
| endpoint | text | 从受控创建结果解析，不能信任任意自报 URL |
| admission | text | active/draining |
| version | bigint | 乐观锁版本 |
| drain_operation_id | text nullable | 当前排空命令幂等键 |
| updated_at | timestamptz | 注册更新 |

无跨服务外键。旧代次上报可作历史但不能覆盖当前。Ready 仅来自环境原生资源，不在此建立自定义健康循环。

#### 约束与索引

- 主键为 `(environment_id, workspace_id, replica_id)`；按 Workspace 查询当前副本。
- `admission` 仅允许 active/draining；`version` 递增用于 CAS。
- 同 incarnation/resource 的重复注册幂等；resource/version 冲突由 Workflow 重读核验。

### agent_instance_history

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| environment_id, workspace_id, replica_id, incarnation_id | text composite PK | 某个已确认启动代次的历史身份 |
| 其余已核验身份字段 | 同 agent_instances | 该代次首次登记的资源、generation、地址、类型与凭证摘要/密钥版本 |

#### 约束与索引

- `(workspace_id, agent_config_id, replica_id, incarnation_id)` 唯一，代次不能跨环境/原生 generation 复用。
- 与当前注册同一事务保存；历史冲突使整个注册回滚。
- 历史记录用于已绑定源的协调，不能凭历史记录取得当前实例授权。

## 能力详细说明

- RegisterInstance 接收 Workflow 已核验的稳定归属与资源身份，CAS 当前注册；同代次重复返回既有记录，同 native generation 的不同 incarnation 或跨 generation 复用 incarnation 均拒绝；新 generation 以 expected_version CAS 登记并保留 admission，不使用自报 URL。
- List/Get 查询 Workspace 当前注册，不以缺失注册推断原 Pod 已终止。
- BeginDrain 校验 expected incarnation/version 和稳定 operation_id；提交 draining 屏障，重复命令不重开准入。
实例授权由 agent_instance_registration Workflow 组合本 Domain 当前登记与 Auth/Control/原生资源事实核验；历史记录不授权过期代次。

## 关联关系

### 依赖的 Domain

无；外部事实作为已验证输入。

### 被依赖的 Domain

无直接聚合依赖，Session 路由经 Workflow 使用实例 ID。

### 参与的 Workflow

- [agent_instance_registration](../../workflow/0010-agent_instance_registration/agent_instance_registration.md)：核验身份与登记。
- [session_replica_routing](../../workflow/0007-session_replica_routing/session_replica_routing.md)：排空及 Session 重绑。
- [agent_workload_provisioning](../../workflow/0009-agent_workload_provisioning/agent_workload_provisioning.md)：创建结果供实例登记使用。

### 关联术语

Agent Instance Registration、Agent Replica、Runtime Instance Incarnation。

注册/准入变化供资源与 Session 协调使用，持久化事实不因发布失败回滚。跨 Control/Auth/环境身份验证在 agent_instance_registration；排空等待、队列重绑在 session_replica_routing，Domain 不决定 Turn 终态。
