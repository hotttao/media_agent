# workflow_validation 变更记录

## 2026-07-21 · 契约统一：删 BatchGetAssetForValidation

- 删除 §与 media_materials 契约里的 `BatchGetAssetForValidation` RPC（返回完整 `*Asset` 实体，与"不直读 materials 库"原则冲突，且与 §3 已用的 `AssetReachability` 目的重复）。
- 统一可达性契约为 `AssetReachability`（material / asset）+ `CheckProductAccessible` / `CheckVirtualIPAccessible`（product / virtual_ip），全部指向 [media_materials reachability workflow](../../../media_materials/workflow/0001-reachability/reachability.md)（见 [ADR 0006](../../../adr/0006-reachability-layering.md)）。
- 修正文档内部不一致：原 §3 用 `AssetReachability` / `Check*Accessible`，§契约却另列 `BatchGetAssetForValidation`，本次对齐。
- frontmatter dependencies 的 `materials: media_materials.AssetReachability` 改为 `media_materials.reachability`。

## 2026-07-21 · 调用方对齐 bundle 模式

- 确认 media_workflow 采用 bundle 模式：caller（BFF）在 SubmitJob 前调 `media_materials.reachability` 校验可达性 + 组装 bundle；media_workflow 不做可达性校验。
- §3「输入可达性校验」由"media_workflow 调 AssetReachability / Check*Accessible"改为"职责在 caller，media_workflow 不调"；闸 1 仅校验 slot.kind 注册（BatchGetAssetKind），闸 2 校验 bundle ↔ input_schema。
- frontmatter dependencies 由 `media_materials.reachability` 改为 `media_materials.asset_kind`（media_workflow 仅依赖 BatchGetAssetKind）。
- 可达性规则定义保留在 media_materials reachability workflow，由 caller 调用。
