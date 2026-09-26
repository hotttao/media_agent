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

Execution 把一个已编译 Workflow 版本实例化为 Job 与 StepRun，以业务数据库记录权威状态，并由 Conductor 持久编排技术任务。

## 边界

### 做什么
- 保存 Job 输入快照、StepRun 状态和最终业务资源 ID。
- 通过 Outbox 启动对应 Conductor Workflow，以 `job_id` 作为 correlation/idempotency key。
- 幂等应用完成/失败事件，记录候选确认决定，支持带反馈重新生成、失败重试和取消。

### 不做什么
- 不执行抓取或生成，不复制下游任务状态。
- 不保存二进制对象，不建立到其他服务数据库的外键。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 提交执行 | 创建 Job 与 StepRun | `JobService.SubmitJob` |
| 查询执行 | 返回 Job 与步骤状态 | `GetJob/GetStepRun` |
| 应用步骤结果 | 幂等处理下游完成或失败事件 | `ApplyStepResult` |
| 接受候选 | 对 needs_confirm StepRun 选择唯一候选，待下游确认后推进 DAG | `ConfirmStepRun` |
| 重新生成 | 记录用户反馈并请求下游创建新 attempt | `RegenerateStepRun` |
| 重试执行 | 重置允许重试的步骤 | `RetryStepRun/RetryJob` |
| 取消执行 | 阻止新步骤并请求取消下游任务 | `CancelJob` |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| Job | StepRun、Step Review Decision | 单次执行状态推进 | 每个 StepRun 最终只有一个 output_resource_id；未确认多候选不能解锁后继；每个 review attempt 只能接受一个决定 |

- 下游返回多个 Candidate 不改变 Workflow Step 的单输出定义，且必须进入 needs_confirm。
- 单 Candidate 在 `approval_policy=always` 时进入 needs_confirm；否则自动确认并形成唯一输出。
- `accept(candidate)` 与 `regenerate(feedback)` 是互斥的 Step Review Decision。
- accept 只有在下游确认 Candidate 并完成业务资源登记后，才把 StepRun 置为 succeeded。
- regenerate 保留旧候选审计，递增 StepRun attempt 后重新派发；它不是 failed 状态的 Retry。

## 领域事件

| 事件 | 产生条件 | 关键载荷 | 消费方 |
|:---|:---|:---|:---|
| StepReviewRequested | 用户决定被持久化 | job_id, step_run_id, attempt, decision | Conductor Adapter |
| StepRunSucceeded | 唯一 output_resource_id 已确定 | job_id, step_run_id, output_resource_id | DAG 推进器 |
| StepRunRegenerationStarted | 下游接受重新生成并进入新 attempt | job_id, step_run_id, attempt | 查询投影、UI |

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
| status | text | blocked/pending/dispatching/running/needs_confirm/confirming/regenerating/succeeded/failed/cancelled/skipped |
| resolved_inputs | jsonb | 解析后的资源 ID、Object Path 和常量 |
| conductor_workflow_id | text nullable | Conductor execution ID；不作为业务幂等键 |
| downstream_service | text nullable | 实现 Execution Capability 的服务身份 |
| downstream_task_id | text nullable | Provider/生成任务 ID，不建跨库外键 |
| output_resource_id | text nullable | 最终业务资源 ID |
| failure_code | text nullable | 稳定失败码 |
| attempt | integer | 当前调度次数 |
| approval_policy | text | 从 Workflow Step 快照的审批策略 |
| created_at | timestamptz | 创建时间 |
| finished_at | timestamptz nullable | 终态时间 |

状态：`blocked -> pending -> dispatching -> running -> needs_confirm | succeeded | failed | cancelled`；候选确认走 `needs_confirm -> confirming -> succeeded`，重新生成走 `needs_confirm -> regenerating -> pending`；取消或上游失败可使未开始步骤进入 `skipped`。

### `step_run_reviews`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Step Review Decision ID |
| step_run_id | text | StepRun ID |
| attempt | integer | 被确认的 StepRun attempt |
| decision | text | accept/regenerate |
| candidate_id | text nullable | accept 时的下游 Candidate ID |
| feedback | text nullable | regenerate 时传给下游的用户反馈 |
| decided_by | text | 当前 User ID |
| idempotency_key | text | 用户确认命令幂等键 |
| status | text | pending/applied/failed |
| created_at | timestamptz | 创建时间 |

#### 约束与索引

- `UNIQUE (step_run_id, attempt)`，同一 attempt 只能存在一个有效决定。
- `UNIQUE (step_run_id, idempotency_key)`，重复提交返回同一决定。
- candidate_id 只在 accept 时必填，feedback 只在 regenerate 时使用。

### UUID 生成规则

Job 和 StepRun 使用 ULID；下游幂等键由 `step_run_id + attempt` 派生。

## 能力详细说明

### SubmitJob

1. 校验 Workflow published、Workspace/Scope 一致、material/asset 与 Input Schema 的 `asset_kind` 匹配且输入可达。
2. 事务内保存 Job、全部 StepRun 和 `StartConductorWorkflow` Outbox。
3. Adapter 事务外按 Compilation mapping 启动 Conductor，以 `job_id` 为 correlationId；超时不确定时先按 correlationId 查询，不盲目重启。

Conductor Worker 在返回 COMPLETED 前必须先以 `(step_run_id, attempt, operation)` 幂等提交业务数据库；若确认响应丢失导致重投，重复处理返回已提交结果。Job/StepRun 是 UI 与业务 API 的查询源，Conductor 状态仅用于运维诊断。

### ApplyStepResult

1. 以 event_id 和 downstream_task_id 去重，过期 attempt 直接忽略。
2. 更新当前 StepRun；需要选候选时进入 needs_confirm，否则记录 output_resource_id。
3. 解锁后继 StepRun 或汇总 Job 终态，同时写下一步 Outbox。

### ConfirmStepRun / RegenerateStepRun

1. 校验调用者仍有 Job 所属 Workspace 的执行权限，StepRun 为 needs_confirm，candidate_id 属于当前下游 Task/attempt。
2. 事务内写 Step Review Decision 与 Outbox，并把 StepRun 改为 confirming 或 regenerating；外部调用不在事务内执行。
3. accept 经 `GenerationCandidateConfirmed` 与 Generated Asset 登记后写唯一 output_resource_id、置 succeeded 并推进 DAG。
4. regenerate 经 `GenerationRegenerationAccepted` 后递增 StepRun attempt、回到 pending/running；旧 attempt 的晚到确认不得覆盖新 attempt。
