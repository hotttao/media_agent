# execution domain 变更记录

## 2026-09-28 · Personal 与 Managed Worker 路由

- WorkerDispatch 增加 `execution_target`、`actor_id`、`auth_source`；Managed Worker 的 `credential_id` 为空。
- Personal Worker 使用 API Token 委托身份和 User Domain；Managed Worker 使用 `internal_runtime` 身份和受控 Managed Domain。
- Managed Worker 不读取 Job User 后模拟登录，不请求 Talos；Job User 是业务归属，Worker Actor 是实际执行者。
- 两类 Worker 复用 Dispatch/Result Inbox 幂等链路，但认证入口和 Conductor Domain 严格分离。

## 2026-09-27 · Worker Gateway、Dispatch 与 Task Domain

- 客户端 Media Worker 不再直连 Conductor，改经统一网关与 media_workflow Worker Gateway API 领取、续租和完成任务。
- 新增独立 WorkerDispatch 聚合与 Worker Result Inbox；结果先在本地幂等接收，再异步更新 Conductor。
- Capability Task 在启动时通过 `taskToDomain` 按提交 User 隔离；禁止客户端指定 Domain、无域 fallback 或共享队列领取后再鉴权。
- 客户端只看到 `dispatch_id + attempt`；Conductor task/workflow ID 留在服务端。
- 输入 Object Path 在领取时转换为短时地址，输出先上传受控对象存储，再由 Finalizer 登记唯一 Generated Asset。

## 2026-09-27 · Product 图片进入执行输入

- JobInput 的 ProductData 新增 `image_url`，提交时冻结主图（无主图则首图）的可访问 URL。
- 编译执行时 Product 的 `asset_ref` 使用该 URL，Product ID 继续保留在快照中；图片顺序仍由 Invocation Mapping 决定。

## 2026-09-26 · Conductor HUMAN 审批闭环

- 编译结果冻结 `candidate_count/approval_required`，需要审批的 Step 在 Capability 与 Finalize 之间插入 HUMAN task。
- media_workflow 从 HUMAN input 投影候选与 `needs_confirm`；确认完成 HUMAN task，重新生成从 Capability task 原位 rerun。
- Finalize 完成后由 media_workflow 幂等登记 Generated Asset；客户端 media_worker 不执行审批、Renderer 或 Finalize。

## 2026-07-03 · 初始建模（media_agent v1）
- 由 `media_ai` 的 `job` / `videoTask` 等迁移并合并：
  - 旧版 job 字段 `"job input"` / `"job output"`（JSON）拆成 `jobs.inputs` 入参快照 + `step_runs.resolved_inputs / output_asset_id` 单步快照；不再冗余"整个 workflow 一份 JSON"，理由是 step 粒度快照便于排查与重跑。
  - 状态机由 `pending/running/completed/failed` 扩展为 `succeeded/failed/cancelled`，并独立维护 `started_at` / `finished_at`。
  - StepRun 与 Step 一对一快照模板版本号 → 模板改版后旧 Job 的 Render 仍可重现。
- 「业务规则变了比较多」中提到的"两边都要冗余 job 输入/输出"——本设计通过**只在 execution 域持久化**消除了冗余；workflow 域不存任何 job 维度的快照。

## 修改记录

### 2026-07-21 · 执行协议对齐外部 pull 模型 + 补单步能力
- 行为约束第 2 条由"内部按 step_order 逐个触发"改为"外部 worker pull（ClaimStepRun）+ advanceDAG 推进"，对齐 [workflow_execution](../../workflow/0002-workflow_execution/workflow_execution.md) 与当前实现。
- 确认 `RetryStepRun` / `GetStepRun` 为正式能力（原已列能力清单但 proto 未暴露，本次补全到 proto + workflow_execution，见 [workflow_execution change.md](../../workflow/0002-workflow_execution/change.md)）。
- `FailStepRun` 语义明确：让在跑的前驱 step 跑完，仅后续 cascade skipped（见 workflow_execution §FailStepRun）。
- 跨服务可达性 RPC 统一经 `media_materials.reachability`（[ADR 0006](../../../adr/0006-reachability-layering.md)）。

### 2026-07-21 · bundle 模式对齐
- 行为约束 1 改为 SubmitJob 校验 bundle ↔ input_schema，**不重新校验可达性**（caller 在 SubmitJob 前已调 media_materials.reachability 校验）。
- Retry 权限：可达性不重新校验（bundle 自包含；retry 用原 bundle 或 input_override）。
- 确认 media_workflow 不调 reachability，仅 `BatchGetAssetKind`（asset_kind）+ `UpsertGeneratedAsset`（generated_asset）。
- 与上一条的关系：上一条假设 execution 调 reachability，本次按 bundle 决策校正。

### 2026-07-03 · 第三轮反馈：Job in-place retry 允许
- 加 `retry_count` 与 `retry_history` JSONB 列；新增 `POST /api/v1/jobs/{id}/retry` 与 `POST /api/v1/jobs/{id}/steps/{step_run_id}/retry`。
- 状态机由 "failed 终态" 扩为 `failed → running`（retry 触发）；其余终态不变。
- 上限 `MaxRetries` 默认 3，可在 service 配置。
- 用户反馈明确要求"in-place retry 可以"——旧版"新建 Job"语义保留，但本版补 in-place 通道。
- "旧 Job 重跑"语义改：默认走 retry 路径（旧 attempt 产物可被覆盖）；如要"全新跑"显式用 Submit 新 Job。

## 来自业务规则的强约束
1. `scope` 沿用 workflow.scope，禁止外部修改；提交时强制 workflow.scope == 任务上级 scope（来自调用方传入）。
2. cancel 是**幂等且终态**：running → cancelled 后台任务收尾后允许再写一次 `finished_at`，但不改变 status。
3. Output asset 落 `media_materials.Asset`，仅持 asset_id；产物二进制不在 execution 库，避免大对象影响 DB。
4. retry 时同一 StepRun 行的 `output_asset_id` 会被覆盖；旧 attempt 产物存放在 `retry_history` 与 audit log（不在本 spec 范围）。
5. **单一产出**：StepRun 持 `output_asset_id`（单值），与 Template 单一 `output_resource_type` 对齐；不再支持多产出列表。
# 2026-08-06

- 外部 Worker Pull 改为下游任务命令 + 完成事件；StepRun 只保存下游 service/task ID。
- Job/StepRun 增加 Workspace 边界，删除跨服务数据库外键。

## 2026-09-17 · 单输出下的多候选人工确认

- StepRun 继续只有一个 `output_resource_id`；下游多张图片建模为 Candidate，不扩展为多输出 Step。
- 新增 Step Review Decision，明确 `accept(candidate)` 与 `regenerate(feedback)` 互斥。
- 增加 confirming/regenerating 过渡态，使本地决定、下游 Generation 状态和最终资源登记可以通过 Outbox/Inbox 恢复。
- 重新生成递增 attempt 并保留旧候选；与 failed 后的 Retry 保持不同语义。
- 审批门槛由 Step `approval_policy` 决定：多 Candidate 始终审批；单 Candidate 仅在 `always` 时审批。
## 2026-09-17：Conductor 承担技术编排

- Job/StepRun 继续作为业务事实源，Conductor 只负责持久调度、等待和技术重试。
- SubmitJob 通过 Outbox 以 job_id correlationId 启动编译后的 Definition。
- Capability Worker 必须先幂等提交业务结果，再完成 Conductor Task。

## 2026-09-26：Job 输入按 Asset Kind 选择

- SubmitJob 的 material / asset bundle 必须与冻结 Input Schema 的 `asset_kind` 对齐。
- Job 创建界面消费服务端 Input Schema 生成资源选择器，不再要求业务用户手写 bundle JSON。
