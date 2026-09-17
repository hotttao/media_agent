---
name: workspace
description: Team 所属 Workspace 的业务隔离、生命周期与访问决策
dependencies:
    - media_auth/organization
    - media_auth/team
related_workflow:
    - agent_session_startup
    - workspace_lifecycle
    - runtime_context_assembly
    - team_decommission
---

## 一句话职责

Workspace 负责企业部门下业务资源与运行数据的隔离、可运行状态和访问决策，是其他控制配置与业务资源的归属锚点。

## 边界

### 做什么
- 在一个 active Organization 的 Team 下创建、查询、更新和删除 Workspace。
- 执行 start、stop、restart、rebuild 生命周期命令。
- 基于 Organization 状态、Team Membership、Team 有效功能与 Workspace 状态判断 action。

### 不做什么
- 不保存 Agent Session 或媒体资源。
- 不代理 Workspace 内服务流量。
- 不拥有 Organization、Team Membership 或 Team Entitlement，企业和部门事实归 `media_auth`。
- 不把 Workspace 当作企业或部门；一个 Team 可以拥有多个 Workspace。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理 Workspace | 创建、查询、更新和删除业务隔离边界 | `WorkspaceService.Create/Get/List/Update/Delete` |
| 控制生命周期 | 改变 Workspace 期望运行状态 | `Start/Stop/Restart/Rebuild` |
| 检查访问 | 批量或单个判断用户对 Workspace 的 action | `CheckWorkspaceAccess/BatchCheckWorkspaceAccess` |

## 数据模型

### `workspaces`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Workspace ID，跨服务稳定引用 |
| organization_id | text | 所属 Organization ID，不建跨服务外键；创建后不可变 |
| team_id | text | 所属 Team ID，不建跨服务外键；创建后不可变 |
| created_by | text | 创建 Workspace 的 User ID，仅作审计，不代表组织所有权 |
| name | text | 显示名称 |
| desired_state | text | 期望状态：stopped/running/deleted |
| version | bigint | 乐观锁版本，每次写入递增 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### UUID 生成规则

`workspace_id = ULID()`，由 Control 创建并全局唯一。

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| Workspace | Workspace 状态与组织归属 | 单次 Workspace 配置或状态变更 | 必须属于一个 Team；Team 必须属于同一 Organization；organization_id/team_id 创建后不可变 |

## 能力详细说明

### CheckWorkspaceAccess

1. 按 `workspace_id` 加载真实 `organization_id`、`team_id` 与状态。
2. 向 Auth 查询 Organization active、调用者 Team Membership 及该 Team 对 action 所需功能的有效授权。
3. 企业管理员在其企业授权范围内具有管理能力；Team admin/member 按 action policy 判断。
4. 状态或功能授权不允许该 action 时拒绝；资源服务仍需执行本地资源授权。

Organization/Team 查询失败时 fail closed。Workspace 创建时验证 Team 属于 Organization；由于 Team 不允许迁移 Organization，后续不需要跨服务外键维持该不变量。

创建事务提交前必须确认 Team 仍为 active；decommissioning/decommissioned Team 一律拒绝创建 Workspace。Team Decommission Workflow 只在没有非 deleted Workspace 时提交终态，从而避免部门停用后遗留可运行 Workspace。
