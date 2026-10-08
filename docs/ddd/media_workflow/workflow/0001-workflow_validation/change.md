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
# 2026-08-06

- 输入可达性校验由 media_workflow 在 SubmitJob 内编排，不再信任 BFF 预先组装的 bundle。

## 2026-09-26 · Input Schema 冻结 Asset Kind

- Input Schema 为 material / asset 类型增加 `asset_kind`，使调用方能选择兼容资源而无需猜测 input name。
- 同名 Workflow Input 绑定到多个不同 material/asset kind 时拒绝 `INPUT_ASSET_KIND_CONFLICT`。
- product / virtual_ip 仍由每个消费 Slot 自己的 kind 解析，不强制单一 Input Schema asset_kind。
# 2026-10-07 引用权限

候选图验证加入同企业与Template指定版本可读性；发布进一步检查版本发布状态，执行不重新验证源模板ACL。
