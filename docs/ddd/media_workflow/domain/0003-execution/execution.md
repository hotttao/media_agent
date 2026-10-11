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

Execution保存已编译Workflow实例的业务归属、冻结输入、审批和产物关联；Job与StepRun状态是Conductor执行事实的查询投影，不是另一个调度权威。

### 生成结果与节点引用不变量

步骤结果复用只改变新 StepRun 的产物引用，不重写源 StepRun 或旧 Asset，不登记伪造的新生成轮次。当前节点保留复用来源、输入摘要及已有产物 ID；不能声称本节点调用了 Worker。不同的实际生成调用仍遵守每轮独立产物身份。复用查找必须限定当前归属并核对资源权限与对象可读性，缓存不授予权限；未知历史来源不能猜测匹配。并发未命中允许独立生成，不以锁或自建调度去替代 Conductor。

StepRun 是稳定业务节点，一次被接受的 Generation Result 是独立结果身份。每轮登记独立 Generated Asset；StepRun.output_asset_id 仅引用当前选定结果，不通过更新旧 Asset 表达 Retry。能力生成结果身份在同轮投递、Finalize 技术重试及落库重试期间保持不变；新能力生成轮次使用新身份。同键不同结果拒绝，旧结果和已存在产物引用不被覆盖。产物事实由 Materials asset 聚合维护，Execution 只在本地事务中原子维护当前引用与节点成功状态。

## 边界

### 做什么
- 普通 User 提交也必须指定可访问且具有 `use_workflow` 权限的 Workspace；由 Control 派生并冻结企业归属。API Token 绑定企业与请求选择都不能越过真实 Workspace 企业。Personal Worker 领取时必须同时匹配 Job owner、企业及 Conductor 实例；企业归属为空时拒绝交付。
- Agent委托的User、企业和Workspace来自独立验证的请求上下文。提交、查询和状态变更检查当前Workspace的use_workflow授权；列表和资源查询同时限定owner、企业与Workspace。Job查询先授权后同步Conductor状态，没有企业/Workspace事实的旧Job不对Agent放行。
- 保存 Job 输入快照、StepRun 状态和最终业务资源 ID。
- 通过 Outbox 启动对应 Conductor Workflow，以 `job_id` 作为 correlation/idempotency key。
- 按 Capability 冻结的 Personal/Managed Execution Domain 领取 Conductor Task，持久化WorkerDispatch授权关联；结果直接回写原生Task。
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
| 回写 Worker 结果 | 校验身份与精确Task，直接代理原生结果更新与确认 | `CompleteWorkerTask/FailWorkerTask` |
| 应用步骤结果 | 幂等处理下游完成或失败事件 | `ApplyStepResult` |
| 接受候选 | 对当前原生HUMAN输入选择候选并保存决定，由Conductor推进 | `ConfirmStepRun` |
| 重新生成 | 将反馈提交精确HUMAN，由Conductor循环开启新轮 | `RegenerateStepRun` |
| 重试执行 | 两个入口均代理所属Conductor实例的原生retry，不限定单节点范围 | `RetryStepRun/RetryJob` |
| 取消执行 | 阻止新步骤并请求取消下游任务 | `CancelJob` |
| 导入未完成执行 | 从完整旧图片谱系建立待继续 Job 与已完成 StepRun，不触发 Conductor | `ImportIncompleteExecution` |
| 导入已完成执行 | 从一条完整旧 Video 谱系建立终态 Job 与四个历史成功 StepRun，不触发 Conductor | `ImportCompletedExecution` |
| 继续导入执行 | 补齐缺失输入，原子进入运行态并发出启动请求 | `ResumeExecution` |

## 聚合与不变量

原生 Job 的初次启动参数是 Execution 聚合的不可变事实，随 Job/StepRun 一起提交。启动记录的 `queued→uncertain→started` 与 Job 的 pending/running 区分；不确定发送不允许自动回 queued。每个原生 Job 只能有一条初始启动记录，绑定实例与 started 必须同事务完成，晚到或过期租约不能覆盖当前实例或取消状态。外部调用由 Workflow 协调。

恢复端口接收完整ExecutionStart，而不是只有名称/correlation；关联查询的唯一候选必须核对精确实例的Definition名称/版本、冻结输入与任务Domain才可绑定。重复/异源/缺字段不能视为原命令成功，也不能授权终止异源实例。

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| Job | StepRun、Step Review Decision | 单次执行状态推进 | 每个 StepRun 最终只有一个 output_resource_id；未确认多候选不能解锁后继；每个 review attempt 只能接受一个决定 |
| WorkerDispatch | 授权关联与交付记账 | 本地授权身份不变 | Dispatch绑定原生Task、Capability和Worker身份；原生Task是终态输出权威，不保存新的结果Inbox |

- 下游返回多个 Candidate 不改变 Workflow Step 的单输出定义，且必须进入 needs_confirm。
- 单 Candidate 在 `approval_policy=always` 时进入 needs_confirm；否则自动确认并形成唯一输出。
- Product 输入快照同时包含 Product ID 与执行时选中的图片 URL；`asset_ref` 指向图片而非 Product ID，避免把业务标识传给图片生成能力。
- `accept(candidate)` 与 `regenerate(feedback)` 是互斥的 Step Review Decision。
- regenerate 反馈最多 8192 字节，按原文冻结并参与命令摘要与幂等比较。确认不能携带反馈；目标定义必须声明支持反馈才可接受非空反馈。反馈只影响本节点紧邻下一轮，不修改 Template Version 或 Invocation Mapping。
- accept 只有在下游确认 Candidate 并完成业务资源登记后，才把 StepRun 置为 succeeded。
- 已提交决定以不透明 Review ID 和投递状态作为回执；`pending` 不表示仍可再次审批，`applied` 不表示步骤已产出业务资源。同键异内容冲突，显式同键重放在任务进入下一轮或终态后仍返回原决定，且仍受当前权限约束。
- regenerate保留旧候选审计，通过HUMAN决定与DO_WHILE开启新轮，不递增本地attempt来调度任务。
- Step Retry只是所属实例原生retry的入口，可恢复多个失败/取消Task。成功任务及其产物由Conductor保留；业务不计算影响集合、不挑选renderer、不重置节点。原生失败继续不改变输入或本地rerun周期，StepRun技术retry_count取自当前能力Task。
- 同一节点的新一轮生成与能力 Task 的失败重试分别识别；重新生成不增加失败 `retry_count`。审计保留旧轮，当前候选、确认状态与输出只能来自最新轮；其他并行节点及已完成上游不改变。
- WorkerDispatch 是独立聚合，避免长轮询、心跳和高频租约更新锁住 Job；它只保存 Job/StepRun ID，不直接修改 Job 内部对象。
- 任务归属必须在 Conductor poll 前通过 Execution Domain 隔离，禁止先从共享队列领取再检查 owner。
- Personal Domain 从 `submitted_by` 派生；Managed Domain 从受控 Worker Pool 配置派生。Capability Task 必须服从其冻结 Target，不能跨类型或回退到 `NO_DOMAIN`。
- 导入的图片谱系只有完整上游且媒体已摄取时才能成为 `Imported Incomplete Execution`；导入写入前还须由 `media_control` 以目标用户身份核验 Workspace 可访问性及其 Organization 归属。每个历史图片 StepRun 保存原 source ID、原时间、来源归属和已登记的唯一输出，不冒充在当前 Template Version 下生成。
- 导入 Job 初始为 `awaiting_resume`，没有 Conductor 实例；只有 `ResumeExecution` 可使其进入 running。已完成历史 StepRun 不得被普通 Retry、回调或恢复分支重新执行或覆盖。
- 每条完整来源 Video 独立导入为 `succeeded` Job，四个 StepRun 均为带来源的 `succeeded`，且无 Conductor 实例；源视频 ID 是执行幂等键。VideoPush 与多视频成员关系属于独立 Historical Publication 聚合，不属于该 Job 的单输出。完整执行不可 Resume，也不可用普通 Retry 伪造历史再执行。
- `ResumeExecution` 的输入必须满足全部待运行节点；用户未补齐 pose/scene/movement 等输入时保留 `awaiting_resume`，不可派发 Capability。相同 source key 的重复导入与相同 resume idempotency key 的重复命令必须返回原结果。
- 历史 Resume 冻结的媒体输入保存企业命名空间内稳定 Object Path；签名 URL 不进入 Job 输入、StartResume Outbox 或 Conductor 输入，交付 Worker 时才重新解析。Object Path 是技术引用，不绕过原有素材权限校验。
- Text-only Movement Input 冻结非空 `content`、`asset_kind=movement` 与空 `url`，不得制造 Object Path；它仅能用于未把 movement 槽位映射为 Capability `asset_ref` 的已发布 Workflow Version。已有媒体型输入仍按原对象校验与重签规则处理。

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

### `step_result_reuse`

复用证据是 Generation Result 的派生来源读模型，不是新的执行聚合或任务状态机。只在真实 Finalize 已接受且登记产物后记录；同源结果重放不更新记录，另一次实际生成可记录独立证据。

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text | 来源步骤及结果键的稳定证据身份 |
| organization_id | text | 企业隔离 |
| workspace_id | text | 执行归属隔离 |
| owner_user_id | text | 已接受产物拥有者 |
| scope | text | 业务范围 |
| fingerprint | text | 实际生成输入身份摘要 |
| source_job_id | text | 来源执行 |
| source_step_run_id | text | 来源步骤 |
| source_result_key | text | 已接受生成结果的稳定键 |
| asset_id | text | Materials 已登记产物 ID |
| object_path | text | 稳定受控对象引用，文本产物可为空 |
| content | text | 已接受文本内容 |
| asset_kind | text | 产物业务类别 |
| created_at | timestamptz | 证据登记时间 |

主键和 `(source_step_run_id, source_result_key)` 唯一，同键不同事实冲突；匹配索引包含企业、Workspace、owner、Scope 和 fingerprint，按时间降序选择候选。删除来源执行/步骤时清理派生证据；不引用 Materials 数据库，不存签名 URL，不将删除证据当作删除产物。

Worker Artifact Upload不新增聚合表：Dispatch提供任务尝试与执行者绑定，上传对象的不可覆盖事实由Storage原子条件写持有。准备上传只签发当前尝试某个候选的短期写入凭据；不改变Task/Job/Step状态。上传后仍须原生Complete、审批及Finalize，参见[产物上传流程](../../../workflow/0001-worker_artifact_upload/worker_artifact_upload.md)。

### 执行状态权威与业务投影

Conductor拥有Task/Workflow的执行状态、技术retry_count、循环轮次及失败恢复。Execution聚合保存归属、冻结输入/编译关联、业务审批/生成资源关联与查询投影，不自行拥有Task调度和重跑状态机。失败恢复不新建execution_retries聚合/表，不持久化另一套发送租约、重置计划或witness。

普通失败继续保留成功任务和产物；审批重新生成以精确Human Task/循环轮次辨识新结果。Job/StepRun终态缓存不能与引擎已经恢复的状态冲突；旧结果/决定仍按精确Task、Dispatch和生成身份拒绝，不能只由本地RetryCount决定执行资格。取消意图、资源登记和业务幂等仍是本地事实，投影事务不包含外部调用。

Job查询状态含paused：原生PAUSED是非终态，独立于need_confirm；RUNNING恢复后可重新投影，TERMINATED为cancelled而不是技术失败。取消意图与原始状态缓存区分：接受native取消只由当前活动引擎实例与持久初次启动记录共同授权，事务保存取消意图及清理义务，不在事务中Terminate。取消后投影不能复活Job。

明确input_override使用Execution Rerun，不属于Execution Retry。原生rerun确认后，Job事务原子记录新受控Input、历史周期审计并使Step查询缓存失效；取消意图/实例/版本/周期变化拒绝旧提交。旧Generated Assets与Review不删除，缓存写不能控制原生Task队列。生成来源以本轮原生Input为准，不能因本地审计延迟使用旧商品/人物；引擎提交与本地事务之间不承诺原子性。

### `execution_starts`

workflow_input可携服务端冻结的只读追踪关联元数据，不属于Job业务输入、提交去重语义或权限事实，不新增聚合/表。首次接受与完整启动清单摘要一同保存，重放不更换；无效/缺失元数据不能使任务失败或改变原生执行资格。执行状态、Task身份与取消意图仍是唯一准入事实。

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text | 初次启动记录身份 |
| job_id | text | 唯一原生 Job |
| workflow_name | text | 冻结的引擎 Definition 名称 |
| workflow_version | integer | 冻结的引擎 Definition 版本 |
| compilation_hash | text | 冻结的 Definition hash |
| correlation_id | text | 等于 Job ID 的唯一启动关联 |
| workflow_input | jsonb | 包含 job_id 的稳定输入，不保存签名 |
| task_domains | jsonb | 冻结的能力任务执行域 |
| payload_digest | text | 全部启动语义的 SHA-256 摘要 |
| status | text | queued / uncertain / started / cancel_pending / cancelled |
| claim_token | text nullable | 每次领取独立标识 |
| claimed_at | timestamptz nullable | 两分钟租约起点 |
| delivery_attempts | integer | 领取次数，不是业务重试次数 |
| next_attempt_at | timestamptz | 退避后的下一次领取时间 |
| conductor_workflow_id | text nullable | 已确认的原实例 |
| last_error | text | 固定错误码，无上游正文 |
| created_at | timestamptz | 记录创建时间 |
| updated_at | timestamptz | 投递状态更新时间 |

同库 Job 外键 RESTRICT；Job 与 correlation 唯一；状态和租约成对约束；started/cancel_pending 必须有实例，queued/uncertain 不保存实例，cancelled 可保存经终止对账的实例，未发送取消则无实例。取消 Job 与 queued→cancelled 或 started→cancel_pending 同事务，uncertain 保留查询义务；后台清理不得复活 Job。已有记录的降级不得删除启动审计。历史 Resume 不写此表；普通 Retry 需独立定义尝试身份，不能复用初次启动记录。

### `job_submissions`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| submitted_by | text | 经认证的 User |
| organization_id | text | 当前授权企业 |
| workspace_id | text | 当前授权 Workspace |
| idempotency_key | text | 一次提交意图，1–160 个可见 ASCII 字符，不含空格 |
| request_digest | text | 规范化提交语义 SHA-256 |
| job_id | text | 首次接受的唯一 Job，外键 RESTRICT |
| created_at | timestamptz | 首次接受时间 |

提交回执与 Job 接受为同一一致性边界。主键为 User/企业/Workspace/key，Job 唯一；身份和摘要不可变。同键重放不是 Retry，不改变 Job 状态。首次接受必须同事务锁键、二次查询、保存聚合与回执；降级不得丢失回执。

### `jobs`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Workflow Job ID |
| workspace_id | text | 所属 Workspace |
| organization_id | text | 所属企业；普通提交由授权后的 Workspace 派生，Agent 提交还需匹配可信委托企业 |
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
| job_id | text | 所属 Job；事务内校验 StepRun 与 Job 的关联 |
| conductor_workflow_id / conductor_task_id / task_reference_name | text | 精确审批目标；不向客户端暴露 Conductor 身份 |
| generation | integer | 生成轮次；旧无循环审批为 0，循环轮次从 1 开始 |
| accepted_retry_count | integer nullable | 接受时本地历史周期的不可变审计；非负，未知为null，不作执行资格判断 |
| decision | text | accept/regenerate |
| selected_index | integer nullable | accept 时在本轮候选快照内选中的位置 |
| feedback | text nullable | regenerate 时传给下游的用户反馈 |
| decided_by | text | 当前 User ID |
| idempotency_key | text | 用户确认命令幂等键 |
| status | text | pending/applied/failed |
| candidates_snapshot / output_json | jsonb | 被审批候选与不可变技术决定命令；用于重放和内容核对 |
| request_digest | text | 决定内容摘要；同一幂等键不能改变决定 |
| claim_token / claimed_at | text / timestamptz nullable | 本次后台领取的 fencing 身份与租约时间 |
| delivery_attempts / next_attempt_at / last_error | integer / timestamptz / text nullable | 后台投递次数、退避和不含原始上游响应的稳定错误 |
| updated_at / applied_at | timestamptz / timestamptz nullable | 技术决定提交时间；不等于业务步骤完成时间 |
| created_at | timestamptz | 创建时间 |

#### 约束与索引

- `UNIQUE (step_run_id, conductor_task_id)`，同一实际审批任务只能存在一个有效决定；生成轮和技术失败重试不能混作同一个 attempt。
- `UNIQUE (step_run_id, idempotency_key)`，重复提交返回同一决定。
- selected_index 只在 accept 时必填，feedback 只在 regenerate 时使用。
- Review事务只保存不可变决定，不推进Job/Step执行状态；Repository透传应用层事务，不在锁中调用Conductor。后台写入匹配本次claim token，过期领取者不能覆盖新claim。

accepted_retry_count参与原摘要并保留审计，不与Job缓存计数比较来决定资格。决定只绑定精确HUMAN Task；原生retried标记、Task状态与实例关联防止影响替代任务。同键重放保留原摘要与原审计，未知旧周期仍为null，不回填或删除。

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
| lease_expires_at | timestamptz | 兼容交付刷新时间，不是原生任务截止时间 |
| created_at / delivered_at / completed_at | timestamptz nullable | 生命周期时间 |

约束：同一conductor_task_id+attempt唯一Dispatch，不因本地时间分配新尝试。原生重投给同一绑定身份时保留ID并刷新记账；不重绑定不同Worker/凭证/实例。活动资格由精确原生Task及取消意图决定，不由lease_expires_at判定。

### `worker_result_inbox`

历史存档表，不再由正常Worker回写新增数据或后台扫描交付；现有字段和约束保留，禁止通过删表/删记录完成退役。

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

媒体输入的持久化快照使用企业内 Frozen Storage Reference，不保存临时签名 URL；提交前验证受控来源及可读性。Dispatch 也保存稳定输入，实际交付与同一租约重取时再生成临时媒体 URL；解析失败不能作为可执行输入交付。

1. 校验 Workflow published、Workspace/Scope 一致、material/asset 与 Input Schema 的 `asset_kind` 匹配且输入可达。
2. 事务内保存 Job、全部 StepRun 和 `StartConductorWorkflow` Outbox。
3. Adapter 事务外按 Compilation mapping 启动 Conductor，以 `job_id` 为 correlationId；超时不确定时先按 correlationId 查询，不盲目重启。

启动请求必须携带服务端派生的 `taskToDomain`：Personal Capability 使用 User Domain，Managed Capability 使用受控 Managed Pool Domain。Conductor是运行状态权威；Job/StepRun提供权限过滤后的业务读模型，不能以缓存终态阻止引擎状态刷新。

### PollWorkerTask / HeartbeatWorkerTask

1. Personal 路径由 Gateway 基于 API Token 签发 `sub=User + act.sub=Worker` 的 Internal Identity JWT；本服务校验 audience、scope、Organization Membership/Entitlement 与 Personal Capability allowlist。
2. 服务端把 Capability Version 映射为 Task Definition，并从 Principal 派生 Personal Domain；客户端值不参与路由。Managed Worker 通过内部运行入口只加载 Managed Capability/Domain，不经过该 JWT 路径。
3. 若该 Worker 已有未过期 Dispatch，返回原 Dispatch；否则在对应 Domain 执行 Conductor batch long-poll。
4. poll 成功后先事务写 WorkerDispatch，再返回 `dispatch_id`、清洗后的输入与即时签发的 presigned URL。
5. heartbeat核对精确原生Task，代理IN_PROGRESS/callback并再次确认仍活动，再刷新交付记账；无heartbeat Outbox或本地过期扫描。取消、错误身份、原生终态或替代Task拒绝；本地时间过期不否决活动原Task。

### CompleteWorkerTask / FailWorkerTask

1. Personal结果校验Principal与Dispatch的user、actor、organization、credential、worker instance、attempt；Managed校验可信内部actor、pool、worker instance、attempt；输出遵守Capability schema和Object Path前缀。交付刷新时间不是身份有效期。
2. 查询原生精确Task，核对Job、实例、attempt。已处于匹配终态则直接返回确认；不同终态结果冲突。首次更新要求原Task仍活动且身份归属有效，不使用本地交付时间作为执行截止。
3. 直接提交TaskResult后再查询原Task。状态及输出或失败reason匹配才返回200/submitted；未确认返回安全可重试错误，客户端保留并重发原结果、不重新执行Provider。只更新Dispatch交付记账，不写Inbox/Job/Step。receipt_id为由Dispatch与attempt派生的不透明确认标识，不是新建数据库回执。
4. Conductor调度的Finalize验证本轮结果，以原生实例、StepRun与成功invoke Task身份派生稳定生成结果键，幂等登记Generated Asset并回写原Finalize Task；查询只投影资源引用与状态，不推进DAG。

### Worker失败分类与结果接收

FailWorkerTask的`failure_kind`仅允许空、`permanent`或`transient`，空表示兼容的永久失败。transient映射FAILED，permanent/空映射FAILED_WITH_TERMINAL_ERROR；重放核对原生失败状态及reason，不能把不同分类/原因视为已确认。

没有结果接收事务；原生结果确认后才写WorkerDispatch的status=result_received与completed_at，作为交付记账。completed_at不是整个Workflow完成时间。不修改Job/StepRun、不登记素材；原生Task Definition决定技术重试，未知分类在外部调用前拒绝。历史Inbox只存档，当前结果来源为Conductor。

### ApplyStepResult

结果按精确Conductor Task与生成身份更新读模型，不解锁后继或决定Job完成。后继调度与Workflow终态由Conductor负责；资源登记在引擎调度的Finalize Worker中完成，查询不产生资源副作用。

Finalize以成功invoke的Task ID派生Generation Result键，在Materials幂等登记后才报告原Finalize Task完成。它不写Job/StepRun状态；查询仅投影该Task返回的真实Generated Asset ID，不再次Upsert，不制造result_占位身份。登记重放、新轮独立产物与取消晚到资产遵守Materials的既有幂等边界。

### ConfirmStepRun / RegenerateStepRun

1. 校验调用者仍有 Job 所属 Workspace 的执行权限，StepRun 为 needs_confirm，candidate_id 属于当前下游 Task/attempt。
2. 事务内写 Step Review Decision 与 Outbox，并把 StepRun 改为 confirming 或 regenerating；外部调用不在事务内执行。
3. accept 经 `GenerationCandidateConfirmed` 与 Generated Asset 登记后写唯一 output_resource_id、置 succeeded 并推进 DAG。
4. regenerate 经 `GenerationRegenerationAccepted` 后递增 StepRun attempt、回到 pending/running；旧 attempt 的晚到确认不得覆盖新 attempt。
