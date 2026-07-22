# media_gen 生成执行 / 视频发布上下文

作为 `media_workflow` 的外部 worker，按 `platform` / `gen_way` 拉取 `StepRun` 实际执行生成（mock：产假候选 url/content）并回写候选；将生成产物（视频）发布到目标平台（mock：返回发布状态/url）。本身无持久化状态，全部 job 状态归 `media_workflow`；归 [media_auth/CONTEXT.md](../media_auth/CONTEXT.md) 全局鉴权。对外以 Hertz + Protobuf IDL 暴露 worker 管理 + publish RPC，不暴露 job claim RPC（claim 走 `media_workflow`）。

## 业务术语词典

### 生成执行

**Generation Worker（生成 worker）**:
media_gen 内部轮询 `media_workflow.ClaimStepRun` 的执行单元，按自身声明的 `platform` / `gen_way` / `gen_type` 匹配领取 StepRun；执行生成（mock）后调 `media_workflow.CompleteStepRun` / `FailStepRun` 回写结果。同一进程可启多个 worker（worker_id 区分）。
_禁用替代词_：runner、executor、processor。

**Worker Identity（worker 身份）**:
`(worker_id, platform, gen_way, gen_type)` 元组，作为 Claim 的匹配键与生命周期管理句柄。`worker_id` 由调用方（运维 / StartWorker 调用者）指定，进程内唯一。
_禁用替代词_：worker name、worker tag。

**Worker State（worker 运行态）**:
worker 的进程内状态机：`STOPPED -> RUNNING -> STOPPED`（异常退出可由调用方重新 StartWorker）。仅进程内可见，不持久化；进程重启后所有 worker 默认 STOPPED，需重新 Start。
_禁用替代词_：worker status（避免与 StepRun.status 混淆）。

**Candidate（候选产物）**:
单次 StepRun 执行产出的可选结果之一。`needs_confirm=true` 时产 N 候选交用户选；否则产 1 个 `is_primary=true` 的主候选。结构对齐 `media_workflow.job.Candidate`：`(index, url, content, is_primary)`。
_禁用替代词_：variant、option、output。

**Primary Candidate（主候选）**:
`needs_confirm=false` 的 StepRun 产出的唯一候选，`is_primary=true`，直接作为该 StepRun 的最终产物回写。
_禁用替代词_：default candidate、selected candidate（后者属用户已选，非本服务产出语义）。

**Generation Execution（单次生成执行）**:
worker 从 Claim 到 Complete/Fail 的一次完整执行：Claim 拿到 rendered_prompt + input_assets -> 调下游生成器（本期 mock）-> 产候选 -> Complete/Fail。是 worker loop 的一个迭代单元。
_禁用替代词_：generation attempt、generation run。

### 发布

**Publish（视频发布）**:
将一个已生成的视频 `generated_asset` 发布到目标平台（mock：返回假发布 url + 状态）。发布动作独立于 generation，由调用方（BFF / 编排层）在用户确认最终产物后显式触发。
_禁用替代词_：distribute、deploy、post。

**Publish Target（发布目标）**:
发布动作的目标平台标识（如 `xhs` / `douyin` / `bilibili`），与 `media_crawler` 平台概念对齐。本期 mock 不真实发布。
_禁用替代词_：channel、destination。

**Publish State（发布状态机）**:
单次发布动作的状态：`PENDING -> PUBLISHING -> (SUCCEEDED | FAILED)`。本期 mock 直接从 PENDING 跳到 SUCCEEDED 并返回假 url；真实实现需异步轮询平台状态。
_禁用替代词_：publish status（避免与 Worker State 混淆）。

### 跨服务协作

**Claim / Complete / Fail（StepRun 三段协议）**:
`media_workflow.job.JobService` 暴露的 worker pull 协议，本服务作为客户端调用：`ClaimStepRun` 领取 -> 本地执行 -> `CompleteStepRun`（带候选）或 `FailStepRun`（带 reason）回写。详见 [media_workflow execution](../media_workflow/domain/0003-execution/execution.md)。
_禁用替代词_：lease/commit、acquire/release。

**Worker Loop（worker 轮询循环）**:
worker 进程内的 Claim -> Execute -> Complete 主循环，周期性 poll `ClaimStepRun` 直到无待领 StepRun 或被 `StopWorker` 停止。本期 stub（接口就绪，不实际跑 loop）。
_禁用替代词_：poll loop、claim cycle。
