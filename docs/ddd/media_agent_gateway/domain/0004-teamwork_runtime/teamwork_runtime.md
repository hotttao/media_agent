---
name: teamwork_runtime
description: Teamwork 拓扑版本与运行中 Agent Session 的关联
dependencies:
    - agent_session
    - agent_task
    - media_control/teamwork
related_workflow:
    - teamwork_runtime_change
---

## 一句话职责

Teamwork Runtime 负责把版本化协作拓扑实例化为 Agent Session 关联，并控制拓扑变化后的新任务准入。

## 边界

### 做什么
- 为 Teamwork Turn 创建成员 Session 关联。
- 记录每个运行实例使用的 topology version。
- 拒绝旧版本新任务并取消尚未开始的任务。

### 不做什么
- 不修改成员和协作边定义。
- 不强制终止已经开始的 Turn。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 创建协作 Turn | 按拓扑版本建立 Session Link | `TeamworkRuntimeService.CreateTurn` |
| 查询运行会话 | 返回拓扑实例关联 | `ListSessions` |
| 应用拓扑变更 | 更新新任务准入版本 | `HandleTeamworkTopologyChanged` |
| 校验任务版本 | 只允许当前 Allowed Topology Version 的新任务 | `AllowsTask` |

## 数据模型

### `teamwork_session_links`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Teamwork Session Link ID |
| topology_id | text | Control 中的 Topology ID |
| topology_version | bigint | 实例化时使用的拓扑版本 |
| teamwork_turn_id | text | 一次 Teamwork Turn ID |
| member_id | text | 拓扑成员 ID，同一 Turn 内唯一 |
| session_id | text | 对应 Agent Session ID |
| status | text | active/finishing/finished/cancelled |
| created_at | timestamptz | 创建时间 |
| finished_at | timestamptz nullable | 自然结束时间 |

### UUID 生成规则

Link 与 Teamwork Turn 使用 ULID，Control 的 topology ID/version 原样引用。

### `teamwork_topology_versions`

保存每个 `topology_id` 的不可变 Workspace 归属、Allowed Topology Version 与 active/deleted 状态；版本只能单调递增，后续事件不得转移归属。新 Task 的 `workspace_id` 必须与该归属一致。

### `teamwork_topology_inbox`

以 Control `event_id` 为主键保存已应用事件，保证至少一次投递不会重复产生取消副作用。

## 能力详细说明

### HandleTeamworkTopologyChanged

1. 按 topology ID/version 幂等消费事件。
2. 将旧版本标记为不接受新 Task，取消尚未领取的任务。
3. running Link 保持到 Turn 自然结束，再进入 finished。

Agent Task 仅在显式携带 `topology_id + topology_version` 时受该准入规则约束；两个字段必须同时存在。事件应用事务只取消旧版本的 `queued/retry_wait` Task，已经进入 `running/waiting_turn` 的 Task 与 Turn 自然结束。
