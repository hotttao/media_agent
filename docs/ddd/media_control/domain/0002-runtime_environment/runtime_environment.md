---
name: runtime_environment
description: Agent Runtime 环境登记、授权与 Workspace 放置
dependencies:
    - workspace
related_workflow:
    - agent_session_startup
    - workspace_lifecycle
    - runtime_context_assembly
---

## 一句话职责

Runtime Environment 负责可用执行环境及 Workspace Placement 的控制面事实。

## 边界

### 做什么
- 管理环境连接配置、容量与可用状态。
- 保存环境连接与 Placement/目录引用，供 Gateway 选择创建方式；Kubernetes 副本状态由 Gateway 读取原生资源。
- 计算并保存 Workspace Placement。
- 为 Placement 绑定 Workspace 独占的持久化运行目录引用。
- 提供运行环境 Token/Grant 的控制接口。

### 不做什么
- 不直接创建 Kubernetes/Docker 资源；Gateway 提交 Kubernetes manifest 或调用 Docker 创建服务。
- 不选择或修改 Session 副本绑定；该路由事实属于 Gateway。
- 不以健康探针判断 Turn 终态或业务队列排空；业务执行状态通过 Gateway 从 Agent 取得。
- 不保存 Runtime 内部进程状态或 Agent Session。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理环境 | 维护 Runtime Environment | `RuntimeEnvironmentService.Create/Get/List/Update/Delete` |
| 管理授权 | 维护环境 Grant 与连接 Token | `ListGrants/Grant/Revoke/IssueToken` |
| 查询放置 | 返回 Workspace 当前 Placement | `GetPlacementStatus` |
| 重新放置 | 根据约束选择目标环境 | `ReconcilePlacement` |
| 绑定运行目录 | 为 Workspace 幂等取得持久化目录并固定容器挂载身份 | `EnsureWorkspaceRuntimeDirectory` |

## 数据模型

### `runtime_environments`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Runtime Environment ID |
| name | text | 环境名称 |
| endpoint | text | 环境连接地址：Kubernetes 接入或 Docker 创建服务地址 |
| capacity | jsonb | CPU、内存和并发容量描述 |
| status | text | active/draining/offline |
| version | bigint | 乐观锁版本 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### `environment_grants`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Grant ID |
| environment_id | text | 所属 Runtime Environment |
| subject_id | text | 获得权限的用户或团队 ID |
| subject_type | text | user/team |
| permission | text | use/admin |
| created_at | timestamptz | 创建时间 |

### `workspace_placements`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| workspace_id | text PK | 被放置的 Workspace |
| environment_id | text | 目标 Runtime Environment |
| status | text | pending/placed/failed/unplaced |
| reason | text nullable | 失败或未放置原因 |
| volume_ref | text nullable | Workspace Runtime Directory 的环境内稳定引用 |
| version | bigint | Placement 版本 |
| updated_at | timestamptz | 最后对账时间 |

### `environment_tokens`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Token 元数据 ID |
| environment_id | text | 唯一绑定的 Runtime Environment |
| name | text | Runner/轮换用途标签 |
| token_hash | text | 原始 Token 的 SHA-256；不对外返回 |
| created_by | text | 签发操作用户，仅用于审计 |
| created_at | timestamptz | 签发时间 |
| revoked_at | timestamptz nullable | 撤销时间；非空后验证立即失败 |

### UUID 生成规则

环境和 Grant 使用 ULID；Placement 以 `workspace_id` 唯一。

## 能力详细说明

### ReconcilePlacement

1. 加载 Workspace 期望状态和可用环境。
2. 按容量与授权选择环境；无候选时记录 `unplaced`。
3. 使用版本锁写入 Placement 并发布变更事件，不直接调用 Runtime。

### Workspace Runtime Directory 不变量

- `workspace_id` 与 `volume_ref` 一一对应；跨 Workspace 不复用同一物理目录。
- Agent 容器内挂载点固定为 `/workspace`，宿主目录结构由环境资源创建实现决定；本地 Docker adapter 使用 `/workspace/<workspace_id>`。
- Agent `HOME` 固定在 `/workspace/.home`，使原生会话、用户级配置和 Workspace 成果共享同一持久化边界；切换 Agent 类型不更换该边界。
- 同一 Workspace 的多个副本共享同一持久化运行目录及 HOME；Kubernetes 使用 Workspace 级 RWX PVC，副本通过 StatefulSet ordinal 和 Headless Service 保持稳定网络身份，不使用每副本 volumeClaimTemplates 隔离 Workspace 文件。
- 执行互斥范围是 Session：同一 Session 同时最多一个 running Turn，不同 Session 可在不同副本并行。共享 Prompt、Skill、配置等派生文件的更新采用幂等生成与原子发布，Session 级互斥不能替代共享文件发布的一致性。
- 目录保存 Agent 会话文件、Workspace 文件和可恢复运行配置；数据库中的 Turn、Message、Universal Event 不复制到目录。
- stop/restart 保留目录；删除与 rebuild 的清理必须遵循 Workspace Lifecycle 的显式语义，禁止容器退出时级联删除。
- 目录内保存不可变 mount identity。Agent 配置的 workspace_id 与 identity 不一致时启动失败，不自动改写。

### Environment Token 不变量

- Token 使用 `env_` 前缀和 256-bit 随机值，原始值只在签发响应出现一次，数据库只保存 SHA-256。
- Environment Credential 验证只解析为固定 `environment_id`，不映射为创建用户，也不继承用户或 API Token 权限。
- 同一环境允许新旧 Token 短期并存；Runner 切换到新 Token 后撤销旧 Token，实现无中断轮换。
- 列表只返回 active Token 元数据；撤销幂等，环境删除时 Token 由外键级联删除。

### 副本与环境范围

Kubernetes 对齐 agent-platform 的 StatefulSet/Headless/共享 RWX 方式，支持扩缩容。Gateway 读取 Kubernetes 副本状态、管理 Session 绑定及业务排空；StatefulSet 维护实际副本，kubelet 执行探针。自定义 Runtime Controller 仅创建/协调承载资源，不探活 Agent、不查询 Turn。

扩容不改变已有有效绑定；缩容时 Gateway/Agent 完成当前 running，queued 保持原 FIFO 并重绑继续。Docker 本期只创建单个容器并直接通信，不考虑扩缩容。

## Agent Workload Target

独立聚合以 Workspace 为键保存配置/Placement 引用、受控资源模板、replicas、version、创建/更新者与稳定请求摘要。默认首次创建目标1副本；副本更新要求 manage、expected_version 和 idempotency_key，同 key 不同内容拒绝。资源模板来自部署批准的类型/镜像/Secret 引用，不接受任意用户环境变量、挂载或 Docker 权限。Gateway 服务查询仍重新校验 Workspace use 与 Placement，读取不把环境已创建/Ready 状态写成配置事实。

本地事务写目标与请求幂等记录；提交后由 Gateway 对账读取并创建资源，通知/资源失败不回滚已提交目标。重要跨上下文协调由 agent_workload_provisioning 承担。目标表只引用 Workspace/配置/环境 ID，不与外部服务共表。
