# execution domain 变更记录

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
