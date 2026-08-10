---
name: execution
description: Workflow 的持久化执行、StepRun 编排与结果汇总
dependencies:
    - workflow
    - template
related_workflow:
    - workflow_execution
    - media_workflow_execution
---

## 一句话职责

Execution 把一个 Workflow 版本实例化为 Job 与 StepRun，并通过下游任务 ID 和集成事件推进 DAG。

## 边界

### 做什么
- 保存 Job 输入快照、StepRun 状态和最终业务资源 ID。
- 为可运行 Step 创建下游命令并记录下游任务 ID。
- 幂等应用完成/失败事件，支持确认、重试和取消。

### 不做什么
- 不执行抓取或生成，不复制下游任务状态。
- 不保存二进制对象，不建立到其他服务数据库的外键。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 提交执行 | 创建 Job 与 StepRun | `JobService.SubmitJob` |
| 查询执行 | 返回 Job 与步骤状态 | `GetJob/GetStepRun` |
| 应用步骤结果 | 幂等处理下游完成或失败事件 | `ApplyStepResult` |
| 确认候选 | 选定候选并推进 DAG | `ConfirmStepRun` |
| 重试执行 | 重置允许重试的步骤 | `RetryStepRun/RetryJob` |
| 取消执行 | 阻止新步骤并请求取消下游任务 | `CancelJob` |

## 数据模型

### `jobs`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Workflow Job ID |
| workspace_id | text | 所属 Workspace |
| scope | text | 不可变业务范围 |
| workflow_id | text | Workflow ID |
| workflow_version | bigint | 执行使用的 Workflow 版本 |
| inputs | jsonb | 已校验的输入快照 |
| status | text | pending/running/succeeded/failed/cancelled |
| submitted_by | text | 提交用户 ID |
| idempotency_key | text | Workspace 内幂等键 |
| created_at | timestamptz | 创建时间 |
| finished_at | timestamptz nullable | 终态时间 |

### `step_runs`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Step Run ID |
| job_id | text | 所属 Job |
| step_key | text | Workflow 内步骤标识 |
| template_id | text | 使用的 Template ID |
| template_version | bigint | 使用的 Template 版本 |
| status | text | blocked/pending/dispatching/running/needs_confirm/succeeded/failed/cancelled/skipped |
| resolved_inputs | jsonb | 解析后的资源 ID、Object Path 和常量 |
| downstream_service | text nullable | 执行该步骤的服务身份 |
| downstream_task_id | text nullable | 下游任务 ID，不建跨库外键 |
| output_resource_id | text nullable | 最终业务资源 ID |
| failure_code | text nullable | 稳定失败码 |
| attempt | integer | 当前调度次数 |
| created_at | timestamptz | 创建时间 |
| finished_at | timestamptz nullable | 终态时间 |

状态：`blocked -> pending -> dispatching -> running -> needs_confirm | succeeded | failed | cancelled`；取消或上游失败可使未开始步骤进入 `skipped`。

### UUID 生成规则

Job 和 StepRun 使用 ULID；下游幂等键由 `step_run_id + attempt` 派生。

## 能力详细说明

### SubmitJob

1. 校验 Workflow published、Workspace/Scope 一致且输入可达。
2. 事务内保存 Job、全部 StepRun 和根步骤 Outbox 命令。
3. Dispatcher 事务外调用目标服务创建任务，并回填 downstream_task_id。

### ApplyStepResult

1. 以 event_id 和 downstream_task_id 去重，过期 attempt 直接忽略。
2. 更新当前 StepRun；需要选候选时进入 needs_confirm，否则记录 output_resource_id。
3. 解锁后继 StepRun 或汇总 Job 终态，同时写下一步 Outbox。
