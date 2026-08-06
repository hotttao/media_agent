---
name: schedule
description: Agent 长期计划与幂等触发记录
dependencies:
    - workspace
    - agent_configuration
related_workflow:
    - agent_schedule_execution
---

## 一句话职责

Schedule 负责何时、以谁的身份、在哪个 Workspace 发起 Agent Task，并记录每次到期触发事实。

## 边界

### 做什么
- 创建、更新、停用和立即运行 Agent Schedule。
- 计算下次触发时间并创建 Schedule Trigger。
- 记录 Trigger 到 Agent Task 的提交结果。

### 不做什么
- 不执行 Agent Task；执行归 Gateway。
- 不把 River Job 当业务事实，不持久化 Platform Token。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 管理计划 | 维护时间规则和执行主体 | `AgentScheduleService.Create/List/Update/Delete` |
| 立即触发 | 创建一次人工 Trigger | `RunNow` |
| 领取到期计划 | Worker 幂等创建 Trigger | `ClaimDueSchedules` |
| 记录提交 | 关联 Gateway Agent Task | `RecordTaskSubmission` |

## 数据模型

### `agent_schedules`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Agent Schedule ID |
| workspace_id | text | 执行所在 Workspace |
| requested_by | text | 被代表的用户 ID |
| agent_config_id | text | 使用的 Agent Configuration |
| rule | text | Cron 或其他时间规则 |
| timezone | text | IANA 时区 |
| status | text | active/paused/deleted |
| next_run_at | timestamptz nullable | 下一次计划时间 |
| version | bigint | 乐观锁版本 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### `schedule_triggers`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Trigger ID |
| schedule_id | text | 来源 Schedule |
| due_at | timestamptz | 本次应触发时间，与 schedule_id 联合唯一 |
| status | text | pending/submitted/skipped/failed |
| agent_task_id | text nullable | Gateway 创建的 Agent Task ID |
| error_code | text nullable | 提交失败错误码 |
| created_at | timestamptz | Trigger 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### UUID 生成规则

Schedule 与 Trigger 使用 ULID；重复到期由业务唯一键去重。

## 能力详细说明

### ClaimDueSchedules

1. River 只携带 `schedule_id` 与 `due_at`。
2. 事务内校验计划 active、创建 Trigger、推进 `next_run_at`。
3. 事务外按全局 Workflow 获取 Platform Token 并提交 Agent Task。
