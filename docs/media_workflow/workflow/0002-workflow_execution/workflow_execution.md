---
name: workflow_execution
description: 外部 worker pull 模式调度 StepRun，用 caller 组装的 bundle 渲染执行，落地产物到 sink step
dependencies:
    - execution
    - template
    - materials: media_materials.asset_kind（BatchGetAssetKind）+ generated_asset（UpsertGeneratedAsset）
    - workflow
---

## 一句话职责

本 workflow 是 execution 域的运行时入口：SubmitJob 接收 caller 组装的自包含 JobInput bundle（含 url/content），由**外部 worker** 通过 Claim/Complete 协议拉取 step_run，按 DAG 顺序按 binding 从 bundle 解析 slot -> 调 [template.Render](../../domain/0001-template/template.md#render-接口) 渲染 prompt，推进 StepRun 状态机，产出时调 `media_materials.UpsertGeneratedAsset` 落 generated_assets，最终把 sink step 的 `output_asset_id` 暴露给调用方。**不调 reachability**（bundle 自包含，见 [media_workflow spec](../../../spec/media_workflow/impl.md) §6）。

## 触发点

- `POST /api/v1/jobs`（SubmitJob）：创建 Job + 初始 StepRun，Job 进入 running。
- **外部 worker** 调 `ClaimStepRun` 拉取可调度 step_run（**pull 模型，非内部 worker**）：worker 按 `platform` / `gen_way` 匹配领取 pending step_run，渲染执行后调 `CompleteStepRun` / `FailStepRun` 回写结果。
- 用户/调用方按需调 `ConfirmStepRun` / `RegenerateStepRun` / `RetryStepRun` / `RetryJob` / `CancelJob` / `GetStepRun`。
- 超时回收由 service 内置 scanner 周期执行（非 worker 职责）。

> 本服务采用**外部 worker pull** + **bundle 自包含**：下游平台 worker 主动 Claim，渲染用 caller 已组装的 bundle（含 url/content），media_workflow 不调 media_materials 做解析（仅 BatchGetAssetKind 校验期 + UpsertGeneratedAsset 产出期）。Claim/Complete/Fail/Confirm/Regenerate/Retry 均为 HTTP RPC（见 [execution](../../domain/0003-execution/execution.md) 能力清单）。

## 阶段

### 1. SubmitJob（创建）
- 读 workflow `(version, steps, bindings)`；校验 `job.workflow_version == workflow.version`，不匹配 -> Job failed (`JobVersionStale`)。
- 校验 bundle（闸 2）：required workflow_input 全覆盖、material[]/asset[] name 唯一、无多余类型、product/virtual_ip 非空（如 required）。
- 创建 Job（status=running，input=bundle 快照）+ StepRun：roots=pending，非 roots=blocked。
- **不调 reachability**（可达性已由 caller 在 SubmitJob 前调 reachability workflow 校验）。

### 2. ClaimStepRun（worker pull）
- 原子领取：`UPDATE step_runs SET status=claimed WHERE status=pending AND job 活跃 AND platform/gen_way 匹配 FOR UPDATE SKIP LOCKED`，返回单个 step_run + 其 template 快照。
- 解析 binding -> ResolvedSlot（见 §3），写 `resolved_inputs`。
- 渲染 prompt（`template.Render`），写 `rendered_prompt`。
- 返回 step_run + 渲染结果（rendered_prompt + input_assets + candidate_count）给 worker 执行。

### 3. 单 step 解析与渲染（per step）
对 step 的每条 binding `(to_input, source)`：
- `source=workflow_input`：从 `jobs.input`（bundle）按 slot.name 取；bundle 已含 url/content（caller 组装时填入）。
- `source=step_output`：从前驱 StepRun `output_asset_id` 取 generated_asset；前驱未 succeeded -> cascade skip。前驱产物的 url/content 从前驱 step_run 的 resolved_inputs / output 取。
- `source=const`：用 `const_value`，跳过解析。
- 解析结果 `{resource_type, id, url, content, ...}` 写入 `step_runs.resolved_inputs[to_input]`。
- 渲染：把 `prompt_body` 的 `{{.name.url}}` / `{{.name.content}}` / `{{.name.key}}` 替换为 bundle/resolved 的值。
- **不调 media_materials**（渲染用 bundle 自包含的 url/content；caller 组装 bundle 时已调 reachability 解析 product/vip 输入 + 拿 url/content）。

### 4. CompleteStepRun（worker 推结果）
- 校验 step_run.status=claimed。
- 存 candidates（生成器返回的候选产物）。
- 若 template 声明 `NeedsConfirm` -> step_run=need_confirm + Job=need_confirm（等用户选）。
- 否则 -> `finalizeStepRun`：调 `media_materials.UpsertGeneratedAsset`（带 job_id / step_run_id / template_id / template_version / source_*_id）落 generated_assets -> 回填 `output_asset_id` -> step_run=SUCCEEDED -> `advanceDAG` -> `checkJobCompletion`。

### 5. FailStepRun
- step_run=failed + failure_reason；Job=failed。
- `advanceDAG`：下游 step cascade=skipped。
- **让在跑的前驱 step 跑完**：不中断已 claimed 的前驱，仅把后续未启动 step 置 skipped。

### 6. ConfirmStepRun（need_confirm -> 选定）
- 校验 step_run.status=need_confirm + candidate index 合法。
- 选定 candidate -> `finalizeStepRun`（UpsertGeneratedAsset + SUCCEEDED + advanceDAG + checkJobCompletion）。

### 7. RegenerateStepRun（need_confirm -> 重新生成候选）
- 校验 step_run.status=need_confirm。
- 清 candidates，step_run=pending，Job=running（worker 重新 Claim 生成新候选）。
- **与 RetryStepRun 的边界**：`Regenerate` 仅用于 need_confirm 态重新生成候选；`RetryStepRun` 用于 failed/skipped 态修复重跑。

### 8. RetryStepRun（重跑单 step + 下游）
- 校验 step_run.status ∈ {failed, skipped}。
- 重置该 step_run=pending + `retry_count + 1`；下游 `binding.source=step_output` 引用本 step 的 step_run 按需重置。
- Job=running；worker 重新 Claim。
- `input_override` 可选覆盖 bundle 的部分值（典型：换 asset url 重跑）；可达性由 caller 在 retry 时重新校验或信任原 bundle，media_workflow 不调 reachability。
- **与 RetryJob 的边界**：`RetryJob` 重置整 Job 的 failed/skipped step；`RetryStepRun` 只重跑指定 step + 其下游。

### 9. GetStepRun（只读）
- 返回单条 step_run 状态 + resolved_inputs + rendered_prompt + output_asset_id + failure_reason。

### 10. 超时回收（scanner）
- service 内置周期 scanner：`claimed` 超 `claim_timeout`（默认 10min，可配）未 Complete 的 step_run -> 回收为 pending（`ReclaimStale`），允许重新 Claim。
- scanner 周期默认 60s，可配。
- 防止 worker 崩溃后 step_run 永久 claimed。

### 11. advanceDAG
- 对每个 blocked step_run：若所有 `source=step_output` 依赖的 step_run=SUCCEEDED -> pending；若任一依赖 FAILED/SKIPPED -> cascade skipped。

### 12. checkJobCompletion
- 若 Job 无 step_run 处于 pending/claimed/need_confirm -> Job=succeeded。
- sink step 的 `output_asset_id` 即 Job 最终产物。

## 终态汇总
- 全部 step succeeded -> Job=succeeded。
- 任一 step failed -> Job=failed（让在跑前驱跑完，后续 cascade skipped），failure_reason 指向首个失败 step_run。

## 错误传播

| 来源 | 处理 |
|:---|:---|
| `Template.Render` 缺 required slot | StepRun failed (ErrRequiredSlotMissing) |
| 下游生成器超时 / 失败 | StepRun failed (ErrDownstream{platform})；可通过 RetryStepRun / RetryJob 重跑 |
| Job version stale | Job failed (JobVersionStale)，无须回滚 StepRun |
| Claim 超时未 Complete | scanner 回收 step_run -> pending，重新 Claim（非失败） |

> 不含 ResolveAssetByKind / AssetRead 错误：渲染用 bundle，不调 reachability；asset 解析的 0/>1 命中由 caller 组装 bundle 时处理。

## 跨服务 RPC 列表

| RPC 方向 | 目的 |
|:---|:---|
| `media_workflow` -> `media_materials` `BatchGetAssetKind` | Workflow 校验期校验 slot.kind / output_asset_kind 合法性（asset_kind domain） |
| `media_workflow` -> `media_materials` `UpsertGeneratedAsset` | StepRun SUCCEEDED 时落 step 产出到 generated_assets 表（带生成记录字段） |

> 不调 reachability：bundle 由 caller（BFF）在 SubmitJob 前组装（caller 调 [media_materials reachability workflow](../../../media_materials/workflow/0001-reachability/reachability.md) 校验可达性 + 解析 + 拿 url/content）。media_workflow execution 只读 bundle。

## 关联 domain
- [execution](../../domain/0003-execution/execution.md)
- [template](../../domain/0001-template/template.md)
- [workflow](../../domain/0002-workflow/workflow.md)

## 关联术语
[Execution](../../CONTEXT.md)、[Job](../../CONTEXT.md)、[Step Run](../../CONTEXT.md)、[Workflow Validation](../../CONTEXT.md)、[Asset Reachability](../../../media_materials/CONTEXT.md)。
