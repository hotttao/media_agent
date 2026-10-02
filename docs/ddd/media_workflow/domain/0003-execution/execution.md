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
- 按 Capability 冻结的 Personal/Managed Execution Domain 领取 Conductor Task，持久化 WorkerDispatch 租约与 Worker Result Receipt。
- 幂等应用完成/失败结果，记录候选确认决定，支持带反馈重新生成、失败重试和取消。

### 不做什么
- 不执行抓取或生成，不复制下游任务状态。
- 不保存二进制对象，不建立到其他服务数据库的外键。
- 不认证原始 API Token，不向客户端暴露 Conductor task/workflow ID，也不允许客户端选择 Task Domain。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 提交执行 | 创建 Job 与 StepRun | `JobService.SubmitJob` |
| 查询执行 | 返回 Job 与步骤状态 | `GetJob/GetStepRun` |
| 领取能力任务 | 按 Worker Principal、Capability 和 Execution Domain 创建或恢复 Dispatch | `PollWorkerTask` |
| 续租任务 | 延长 Dispatch 租约并感知 Job 取消 | `HeartbeatWorkerTask` |
| 接收 Worker 结果 | 校验 attempt/schema/object ref，写 Result Inbox 与稳定收据 | `CompleteWorkerTask/FailWorkerTask` |
| 应用步骤结果 | 幂等处理下游完成或失败事件 | `ApplyStepResult` |
| 接受候选 | 对 needs_confirm StepRun 选择唯一候选，待下游确认后推进 DAG | `ConfirmStepRun` |
| 重新生成 | 记录用户反馈并请求下游创建新 attempt | `RegenerateStepRun` |
| 重试执行 | 重置允许重试的步骤 | `RetryStepRun/RetryJob` |
| 取消执行 | 阻止新步骤并请求取消下游任务 | `CancelJob` |
| 导入未完成执行 | 从完整旧图片谱系建立待继续 Job 与已完成 StepRun，不触发 Conductor | `ImportIncompleteExecution` |
| 继续导入执行 | 补齐缺失输入，原子进入运行态并发出启动请求 | `ResumeExecution` |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| Job | StepRun、Step Review Decision | 单次执行状态推进 | 每个 StepRun 最终只有一个 output_resource_id；未确认多候选不能解锁后继；每个 review attempt 只能接受一个决定 |
| WorkerDispatch | Worker Result Receipt | 单次能力交付与结果接收 | 一个 Dispatch 只绑定一个 Job/StepRun attempt、Capability、Execution Target、User、执行 Actor 和 lease；同一 operation 只产生一个结果收据 |

- 下游返回多个 Candidate 不改变 Workflow Step 的单输出定义，且必须进入 needs_confirm。
- 单 Candidate 在 `approval_policy=always` 时进入 needs_confirm；否则自动确认并形成唯一输出。
- Product 输入快照同时包含 Product ID 与执行时选中的图片 URL；`asset_ref` 指向图片而非 Product ID，避免把业务标识传给图片生成能力。
- `accept(candidate)` 与 `regenerate(feedback)` 是互斥的 Step Review Decision。
- accept 只有在下游确认 Candidate 并完成业务资源登记后，才把 StepRun 置为 succeeded。
- regenerate 保留旧候选审计，递增 StepRun attempt 后重新派发；它不是 failed 状态的 Retry。
- WorkerDispatch 是独立聚合，避免长轮询、心跳和高频租约更新锁住 Job；它只保存 Job/StepRun ID，不直接修改 Job 内部对象。
- 任务归属必须在 Conductor poll 前通过 Execution Domain 隔离，禁止先从共享队列领取再检查 owner。
- Personal Domain 从 `submitted_by` 派生；Managed Domain 从受控 Worker Pool 配置派生。Capability Task 必须服从其冻结 Target，不能跨类型或回退到 `NO_DOMAIN`。
- 导入的图片谱系只有完整上游且媒体已摄取时才能成为 `Imported Incomplete Execution`；导入写入前还须由 `media_control` 以目标用户身份核验 Workspace 可访问性及其 Organization 归属。每个历史图片 StepRun 保存原 source ID、原时间、来源归属和已登记的唯一输出，不冒充在当前 Template Version 下生成。
- 导入 Job 初始为 `awaiting_resume`，没有 Conductor 实例；只有 `ResumeExecution` 可使其进入 running。已完成历史 StepRun 不得被普通 Retry、回调或恢复分支重新执行或覆盖。
- `ResumeExecution` 的输入必须满足全部待运行节点；用户未补齐 pose/scene/movement 等输入时保留 `awaiting_resume`，不可派发 Capability。相同 source key 的重复导入与相同 resume idempotency key 的重复命令必须返回原结果。
- 历史 Resume 冻结的媒体输入保存企业命名空间内稳定 Object Path；签名 URL 不进入 Job 输入、StartResume Outbox 或 Conductor 输入，交付 Worker 时才重新解析。Object Path 是技术引用，不绕过原有素材权限校验。

## 领域事件

| 事件 | 产生条件 | 关键载荷 | 消费方 |
|:---|:---|:---|:---|
| StepReviewRequested | 用户决定被持久化 | job_id, step_run_id, attempt, decision | Conductor Adapter |
| StepRunSucceeded | 唯一 output_resource_id 已确定 | job_id, step_run_id, output_resource_id | DAG 推进器 |
| StepRunRegenerationStarted | 下游接受重新生成并进入新 attempt | job_id, step_run_id, attempt | 查询投影、UI |
| WorkerTaskDispatched | Capability Task 已持久交付 | dispatch_id, step_run_id, attempt, capability, worker_instance_id | 审计、租约扫描器 |
| WorkerResultReceived | Worker 结果已幂等接收 | dispatch_id, attempt, receipt_id, result_digest | Conductor Adapter |
| WorkerDispatchExpired | 租约到期或任务取消 | dispatch_id, step_run_id, attempt | Conductor Adapter、审计 |

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
| status | text | awaiting_resume（仅历史导入）/pending/running/succeeded/failed/cancelled |
| submitted_by | text | 提交用户 ID |
| idempotency_key | text | Workspace 内幂等键 |
| origin | text | native / legacy_import；不得把历史已完成 StepRun 伪装成原生执行 |
| source_system / source_record_id | text nullable | 旧执行谱系的稳定幂等来源键；同 Workspace 唯一 |
| imported_at | timestamptz nullable | 导入发生时间；不覆盖原 created_at |
| created_at | timestamptz | 创建时间 |
| finished_at | timestamptz nullable | 终态时间 |

导入执行还需保存各历史 StepRun 的 `source_system/source_record_id/source_created_at` 与媒体对象摘要；引用同一旧图片的多个 Job 可以复用同一个已登记 Asset，但各 Job 的 StepRun 身份独立。上游缺失或错配的源记录不创建 Job。`awaiting_resume -> running` 只能由当前授权用户显式触发，业务状态由 Job 聚合决定；对象搬运、Conductor 启动和结果回写由 Workflow 协调。

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

### `worker_dispatches`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | 对外不透明 dispatch_id |
| conductor_task_id | text | 内部 Conductor Task ID，不对客户端返回 |
| conductor_workflow_id | text | 内部 Conductor Workflow ID |
| job_id / step_run_id | text | 关联执行与步骤，不建跨聚合对象引用 |
| attempt | integer | 对应 StepRun attempt |
| capability_key / capability_version | text / bigint | 已冻结的执行能力版本 |
| user_id / organization_id | text | 任务所有者与企业边界 |
| execution_target | text | personal_worker / managed_worker |
| actor_id | text | 实际执行 Actor；Personal/Managed 均使用稳定 Worker ID |
| credential_id | text nullable | Personal Worker 的 API Token ID；Managed Worker 为空 |
| worker_instance_id | text | 本次 Worker 进程实例审计标识 |
| auth_source | text | api_token / internal_runtime |
| status | text | claimed/delivered/running/result_received/expired/failed |
| lease_expires_at | timestamptz | 本地租约到期时间 |
| created_at / delivered_at / completed_at | timestamptz nullable | 生命周期时间 |

约束：同一 `conductor_task_id + attempt` 最多一个未过期 Dispatch；同一 Worker 有有效 Dispatch 时重复 poll 返回原 Dispatch，不重复领取。

### `worker_result_inbox`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | 稳定 receipt_id |
| dispatch_id | text | 所属 WorkerDispatch |
| attempt | integer | 客户端提交 attempt |
| operation | text | complete/fail |
| result | jsonb | 已校验的候选、Object Path、摘要和 provenance |
| result_digest | text | 审计与重复结果比较 |
| conductor_status | text | pending/submitted/failed |
| created_at / submitted_at | timestamptz nullable | 接收与提交时间 |

约束：`UNIQUE(dispatch_id, attempt, operation)`；重复请求返回原 receipt。旧 attempt 返回冲突，不覆盖当前执行。

### UUID 生成规则

Job 和 StepRun 使用 ULID；下游幂等键由 `step_run_id + attempt` 派生。

## 能力详细说明

### SubmitJob

1. 校验 Workflow published、Workspace/Scope 一致、material/asset 与 Input Schema 的 `asset_kind` 匹配且输入可达。
2. 事务内保存 Job、全部 StepRun 和 `StartConductorWorkflow` Outbox。
3. Adapter 事务外按 Compilation mapping 启动 Conductor，以 `job_id` 为 correlationId；超时不确定时先按 correlationId 查询，不盲目重启。

启动请求必须携带服务端派生的 `taskToDomain`：Personal Capability 使用 User Domain，Managed Capability 使用受控 Managed Pool Domain。Job/StepRun 是 UI 与业务 API 的查询源，Conductor 状态仅用于运维诊断。

### PollWorkerTask / HeartbeatWorkerTask

1. Personal 路径由 Gateway 基于 API Token 签发 `sub=User + act.sub=Worker` 的 Internal Identity JWT；本服务校验 audience、scope、Organization Membership/Entitlement 与 Personal Capability allowlist。
2. 服务端把 Capability Version 映射为 Task Definition，并从 Principal 派生 Personal Domain；客户端值不参与路由。Managed Worker 通过内部运行入口只加载 Managed Capability/Domain，不经过该 JWT 路径。
3. 若该 Worker 已有未过期 Dispatch，返回原 Dispatch；否则在对应 Domain 执行 Conductor batch long-poll。
4. poll 成功后先事务写 WorkerDispatch，再返回 `dispatch_id`、清洗后的输入与即时签发的 presigned URL。
5. heartbeat 延长本地 lease，并通过 Outbox 异步更新 Conductor；Job 取消、凭证撤销或 lease 过期时拒绝续租。

### CompleteWorkerTask / FailWorkerTask

1. Personal 结果校验 Principal 与 Dispatch 的 user、actor、organization、credential/lease、worker instance、attempt；Managed 结果校验可信内部 actor、pool、lease、worker instance、attempt；两者都校验 Capability 输出 schema 和 Object Path 所属前缀。
2. 事务内以 `(dispatch_id, attempt, operation)` 写 Worker Result Inbox、标记 Dispatch `result_received`；立即返回 `202 + receipt_id`。
3. Conductor Adapter 重试提交 TaskResult；结果不明确时按 conductor_task_id 查询，不能要求客户端重新执行 Provider。
4. Finalizer 验证对象元数据，以 `(step_run_id, attempt)` 幂等登记 Generated Asset，再通过 ApplyStepResult 推进业务状态。

### ApplyStepResult

1. 以 event_id 和 downstream_task_id 去重，过期 attempt 直接忽略。
2. 更新当前 StepRun；需要选候选时进入 needs_confirm，否则记录 output_resource_id。
3. 解锁后继 StepRun 或汇总 Job 终态，同时写下一步 Outbox。

### ConfirmStepRun / RegenerateStepRun

1. 校验调用者仍有 Job 所属 Workspace 的执行权限，StepRun 为 needs_confirm，candidate_id 属于当前下游 Task/attempt。
2. 事务内写 Step Review Decision 与 Outbox，并把 StepRun 改为 confirming 或 regenerating；外部调用不在事务内执行。
3. accept 经 `GenerationCandidateConfirmed` 与 Generated Asset 登记后写唯一 output_resource_id、置 succeeded 并推进 DAG。
4. regenerate 经 `GenerationRegenerationAccepted` 后递增 StepRun attempt、回到 pending/running；旧 attempt 的晚到确认不得覆盖新 attempt。
