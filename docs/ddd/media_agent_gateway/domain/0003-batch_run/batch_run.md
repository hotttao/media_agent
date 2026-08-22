---
name: batch_run
description: 一组 Agent Task 的批量提交和汇总状态
dependencies:
    - agent_task
related_workflow:
    - batch_run_dispatch
---

## 一句话职责

Batch Run 负责批量创建 Agent Task、汇总进度并传播取消意图。

## 边界

### 做什么
- 创建 Batch Run 及不可变任务输入项。
- 幂等创建每个 Agent Task。
- 汇总成功、失败、运行和取消数量。

### 不做什么
- 不复制 Agent Task 状态为第二事实源。
- 不控制单个 Task 的重试策略。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 创建批次 | 保存输入项并创建任务 | `BatchRunService.Create` |
| 查询批次 | 返回动态汇总状态 | `Get/List` |
| 取消批次 | 向未终态任务传播取消 | `Cancel` |

## 数据模型

### `batch_runs`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Batch Run ID |
| workspace_id | text | 执行所在 Workspace |
| requested_by | text | 发起用户 ID |
| status | text | creating/running/completed/partial_failed/failed/cancelled |
| item_count | integer | 固定输入项数量 |
| next_cursor | integer | 已完成 Task 拆分的稳定游标 |
| completed_count/failed_count/cancelled_count | integer | Item 终态投影计数 |
| idempotency_key | text | Workspace 内幂等键 |
| created_at | timestamptz | 创建时间 |
| finished_at | timestamptz nullable | 批次终态时间 |
| updated_at | timestamptz | 最近状态更新时间 |

### `batch_run_items`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| batch_run_id | text | 所属 Batch Run |
| item_key | text | 批次内唯一项标识，与 batch_run_id 联合主键 |
| session_id | text | 已验证 owner/workspace 的 Agent Session |
| agent_task_id | text nullable | 已创建的 Agent Task ID |
| input | jsonb | 该项不可变输入 |
| status | text | pending/queued/running/completed/failed/cancelled 投影 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最近状态更新时间 |

### UUID 生成规则

Batch Run 使用 UUID v4；Task 幂等键由 `batch_run_id + item_key` 派生。

## 能力详细说明

### Create

1. 以客户端幂等键创建 Batch Run 与固定输入项。
2. Outbox/Worker 为每项创建 Agent Task，重复执行复用同一 Task。
3. Batch 状态由 Task 终态投影计算，不反向修改 Task 结果。
