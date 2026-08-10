---
name: workspace
description: Workspace 租户边界、生命周期与访问决策
dependencies:
    - media_auth/team
related_workflow:
    - agent_session_startup
    - workspace_lifecycle
    - runtime_context_assembly
---

## 一句话职责

Workspace 负责平台租户边界、owner 和可运行状态，是其他控制配置与业务资源的归属锚点。

## 边界

### 做什么
- 创建、查询、更新和删除 Workspace。
- 执行 start、stop、restart、rebuild 生命周期命令。
- 基于 owner 与 Team Membership 判断 Workspace action。

### 不做什么
- 不保存 Agent Session 或媒体资源。
- 不代理 Workspace 内服务流量。
- 不拥有 Team Membership，团队事实归 `media_auth`。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理 Workspace | 创建、查询、更新和删除租户边界 | `WorkspaceService.Create/Get/List/Update/Delete` |
| 控制生命周期 | 改变 Workspace 期望运行状态 | `Start/Stop/Restart/Rebuild` |
| 检查访问 | 批量或单个判断用户对 Workspace 的 action | `CheckWorkspaceAccess/BatchCheckWorkspaceAccess` |

## 数据模型

### `workspaces`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Workspace ID，跨服务稳定引用 |
| owner_user_id | text | 所有者 User ID，不建跨服务外键 |
| name | text | 显示名称 |
| desired_state | text | 期望状态：stopped/running/deleted |
| version | bigint | 乐观锁版本，每次写入递增 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### UUID 生成规则

`workspace_id = ULID()`，由 Control 创建并全局唯一。

## 能力详细说明

### CheckWorkspaceAccess

1. 按 `workspace_id` 加载真实 owner 与状态。
2. owner 直接拥有管理权限；否则批量查询 Auth Team grant。
3. 状态不允许该 action 时拒绝；资源服务仍需执行本地资源授权。
