# generation domain 变更记录

## 2026-07-21 · 初始建模（media_gen v1）

- 新建 generation domain：作为 `media_workflow` 外部 worker，调 `ClaimStepRun` / `CompleteStepRun` / `FailStepRun` 三段协议，本地 mock 产候选。
- 明确**不持久化**：worker 是无状态执行单元，全部 job 状态归 `media_workflow`。回退成本低（加表即可），未达 ADR 阈值。
- 明确**不渲染 prompt**：`rendered_prompt` 由 `media_workflow` 在 Claim 时渲染完成，本域直接消费。
- 明确**不落地产物**：产物落 `media_materials.generated_assets` 由 `media_workflow` 在 `CompleteStepRun` 内调 `UpsertGeneratedAsset` 完成，本域只回写候选 url/content。
- mock 候选产出规则：`candidate_count > 1` -> N 候选全 `is_primary=false`；`candidate_count == 1` -> 单候选 `is_primary=true`。
# 2026-08-06

- Generation 从无状态 StepRun Worker 调整为持久化 Generation Task 聚合；media_workflow 只保存下游 task ID，并通过命令/事件协作。
- 候选产物改存 Storage Object Path，不保存完整 URL。

## 2026-09-17 · 多候选确认与反馈重新生成

- 保持 Template/Step 单一最终输出不变量：一次 Generation Task 可以返回多张 Candidate，但必须确认唯一 Primary Candidate 后才形成 Step 输出。
- 新增 Generation Review，区分 `accept(candidate)` 与 `regenerate(feedback)`。
- 重新生成保留旧 Candidate、递增 attempt，并明确不等同于失败重试；同输入的主动重新生成不得复用上一 attempt。
