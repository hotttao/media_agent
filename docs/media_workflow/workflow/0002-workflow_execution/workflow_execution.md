---
name: workflow_execution
description: 拿到已校验的 Job，按 DAG 顺序调度 StepRender，落地 StepRun，最终汇总到 sink step
dependencies:
    - execution
    - template
    - materials: media_materials.AssetRead（拿资源二进制 / URL）
    - workflow
---

## 一句话职责
本 workflow 是 execution 域的运行时入口：拿到一个已 Submit 的 Job，按 step_order 顺序推 step 调度，解析每条 StepBinding 获取 ResolvedSlot，调用 [template.Render](../../domain/0001-template/template.md#render-接口) → 通过 `media_materials.AssetRead` 拿资源二进制 → 提交到下游生成器，落地 StepRun 状态机，最终把 sink step 的 output_asset_id 暴露给调用方。

## 触发点

- Job 进入 running 状态后由内部 worker 拉起（不在 HTTP 层暴露）。

## 阶段

### 1. 启动守卫
读 Job → 取 `workflow_id` + `workflow_version` → 重新加载 Workflow 的 `(version, steps, bindings)`。**版本不匹配**：job 直接转 `failed` (`JobVersionStale`)。

### 2. 拓扑排序
根据 binding 的 step_output 关系反推前驱 step；保证一个 step 的所有前驱 step_run 状态为 succeeded 才能被调度。

### 3. 单 step 渲染与执行（per step）
对每个可调度 step：
1. 解析 Binding，把每个 `(to_input, source)` 实际映射为 `ResolvedSlot`：
   - `source=workflow_input`：取 `jobs.inputs[ref_workflow_input.name]`，按 workflow_input.`type` 分派：
     - `type=material`：得 `material_id`；校验 material.asset_kind == 目标 slot.kind。
     - `type=asset`：得 `generated_asset_id`；校验 generated_asset.asset_kind == slot.kind。
     - `type=product`：得 `product_id` → 调 `media_materials.ResolveAssetByKind(owner_kind=product, owner_id=product_id, asset_kind=slot.kind)`；按 slot.kind.resource_type 分发到 materials / generated_assets 取唯一 asset。
     - `type=virtual_ip`：得 `virtual_ip_id` → 同上 `ResolveAssetByKind(owner_kind=virtual_ip, ...)`。
     - 0 或 >1 个 → StepRun failed（`ErrAssetResolveAmbiguous` / `ErrAssetResolveMissing`）。解析结果 `{resource_type, id}` 写入 StepRun.resolved_inputs[to_input]。
   - `source=step_output`：从前驱 StepRun 的 `output_asset_id`（单一 generated_asset 产出）取；前驱未 succeeded → cascade skip 当前 step。
   - `source=const`：用 `const_value`，跳过 Asset 解析。
2. 把 ResolvedSlot 中每个 (resource_type, id) 调 `media_materials.AssetRead(resource_type, id)` 拿可渲染字符串（典型：取 `url` / `metadata[slot.name]`）。
3. 调 `template.Render(template_version, slots)` → 拿到 `(prompt, output_asset_kind, output_media_type)`。
4. 按 Template.`platform` / `gen_way` / `gen_type` + `output_media_type` 调下游生成器；返回单一产出 → 调 `media_materials.UpsertGeneratedAsset`（带 job_id / step_run_id / template_id / template_version / source_virtual_ip_id / source_product_id）落 `generated_assets` 表。
5. 写 StepRun：status=succeeded、output_asset_id（= 新 generated_asset.id）持久化。

### 4. 终态汇总
- 全部 step succeeded → Job status=succeeded。
- 任一 step failed → 仍在跑的前驱 step 被允许跑完（避免半成品取消更糟糕），但把后续所有 step 置为 skipped，Job status=failed，failure_reason 指向首个失败 step_run.failure_reason。

### 5. 对外暴露
只读查询接口已经在 [execution](../../domain/0003-execution/execution.md) 暴露；本 workflow 不重复。

## 错误传播

| 来源 | 处理 |
|:---|:---|
| `Template.Render` 缺 required slot | StepRun failed (ErrRequiredSlotMissing) |
| `media_materials.ResolveAssetByKind` 0 / >1 命中 | StepRun failed (ErrAssetResolveMissing / ErrAssetResolveAmbiguous) |
| `media_materials.AssetRead` 找不到 asset | StepRun failed (ErrAssetMissing) |
| 下游生成器超时 / 失败 | StepRun failed (ErrDownstream{platform})；可通过 [execution §Retry](../../domain/0003-execution/execution.md#retry) 重跑 |
| Job version stale | Job failed (JobVersionStale)，无须回滚 StepRun |

## 跨服务 RPC 列表

| RPC 方向 | 目的 |
|:---|:---|
| `media_workflow` → `media_materials` AssetReachability | Submit 阶段校验 material / asset 输入可达性（[workflow_validation](../0001-workflow_validation/workflow_validation.md)） |
| `media_workflow` → `media_materials` CheckProductAccessible / CheckVirtualIPAccessible | Submit 阶段校验 product / virtual_ip 输入可达性 |
| `media_workflow` → `media_materials` ResolveAssetByKind | 渲染 step 时把 product / virtual_ip 输入按 slot.kind 解析为具体 material/generated_asset |
| `media_workflow` → `media_materials` AssetRead | 渲染 step 时按 (resource_type, id) 拉资源 |
| `media_workflow` → `media_materials` UpsertGeneratedAsset | 落 step 产出到 generated_assets 表（带生成记录字段） |
| `media_workflow` → `media_materials` BatchGetAssetKind | 校验 slot.kind / output_asset_kind 合法性 |

## 关联 domain
- [execution](../../domain/0003-execution/execution.md)
- [template](../../domain/0001-template/template.md)
- [workflow](../../domain/0002-workflow/workflow.md)

## 关联术语
[Execution](../../CONTEXT.md)、[Job](../../CONTEXT.md)、[Step Run](../../CONTEXT.md)、[Workflow Validation](../../CONTEXT.md)、[Asset Reachability](../../media_materials/CONTEXT.md)。
