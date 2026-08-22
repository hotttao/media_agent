---
name: agent_task
description: 异步 Agent 执行请求及其可重试状态
dependencies:
    - agent_session
related_workflow:
    - agent_schedule_execution
    - agent_task_dispatch
    - batch_run_dispatch
---

## 一句话职责

Agent Task 负责一次异步 Agent 执行请求的持久化、调度、取消和最终结果。

## 边界

### 做什么
- 按幂等键创建 Agent Task。
- 通过 River 分发、续租和重试任务。
- 记录取消、完成与失败结果。

### 不做什么
- 不拥有 Schedule 时间规则。
- River Job 不是 Agent Task 的事实源。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 创建任务 | 持久化异步 Agent 请求 | `AgentTaskService.Create` |
| 查询任务 | 返回任务状态与结果引用 | `Get/List` |
| 取消任务 | 阻止未开始任务或请求运行时停止 | `Cancel` |
| 执行任务 | Worker 领取并推进状态 | `DispatchAgentTask` |

## 数据模型

### `agent_tasks`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Agent Task ID |
| workspace_id | text | 执行所在 Workspace |
| requested_by | text | 被代表的用户 ID |
| session_id | text nullable | 关联 Agent Session |
| source | text | schedule/batch/manual 等任务来源 |
| source_resource_id | text | 来源业务记录 ID |
| idempotency_key | text | 来源内幂等键 |
| input | jsonb | 不含凭据的执行输入快照 |
| status | text | queued/running/waiting_turn/retry_wait/completed/failed/cancelled |
| attempt | integer | 当前执行次数 |
| turn_id | text nullable | Agent 接受任务后关联的唯一 Turn |
| result_ref | jsonb nullable | 结果资源引用 |
| error_code | text nullable | 稳定错误码 |
| error_message | text nullable | 脱敏错误说明 |
| created_at | timestamptz | 创建时间 |
| started_at | timestamptz nullable | 首次开始时间 |
| finished_at | timestamptz nullable | 终态时间 |
| updated_at | timestamptz | 最近状态更新时间 |

状态机：`queued -> running -> waiting_turn -> completed | failed | cancelled`；瞬时投递失败可由 `running -> retry_wait -> running`。终态不可互相覆盖，晚到 Turn 事件不得改写终态。

### UUID 生成规则

`agent_task_id = ULID()`；River payload 只保存 task ID。

## 能力详细说明

### DispatchAgentTask

1. 原子领取 queued/retry_wait Task，递增 attempt；重复领取返回当前状态。
2. 事务外使用 `task_id` 作为幂等键向 Agent 提交 Turn；Agent 决定立即执行或进入持久化 FIFO。
3. Agent 接受后保存唯一 `turn_id` 并进入 waiting_turn，不占用数据库事务等待执行。
4. Turn 终态事件或补偿查询按 task ID、turn ID 与 attempt 幂等收敛；过期 attempt 和晚到事件不覆盖新状态。
