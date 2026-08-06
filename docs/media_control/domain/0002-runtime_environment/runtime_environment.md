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
- 计算并保存 Workspace Placement。
- 提供运行环境 Token/Grant 的控制接口。

### 不做什么
- 不启动 Agent Runtime；启动由 Runtime Controller 执行。
- 不保存 Runtime 内部进程状态或 Agent Session。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理环境 | 维护 Runtime Environment | `RuntimeEnvironmentService.Create/Get/List/Update/Delete` |
| 管理授权 | 维护环境 Grant 与连接 Token | `ListGrants/Grant/Revoke/IssueToken` |
| 查询放置 | 返回 Workspace 当前 Placement | `GetPlacementStatus` |
| 重新放置 | 根据约束选择目标环境 | `ReconcilePlacement` |

## 数据模型

### `runtime_environments`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Runtime Environment ID |
| name | text | 环境名称 |
| endpoint | text | Runtime Controller 内部地址 |
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
| version | bigint | Placement 版本 |
| updated_at | timestamptz | 最后对账时间 |

### UUID 生成规则

环境和 Grant 使用 ULID；Placement 以 `workspace_id` 唯一。

## 能力详细说明

### ReconcilePlacement

1. 加载 Workspace 期望状态和可用环境。
2. 按容量与授权选择环境；无候选时记录 `unplaced`。
3. 使用版本锁写入 Placement 并发布变更事件，不直接调用 Runtime。
