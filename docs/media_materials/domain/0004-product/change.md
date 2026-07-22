# product domain 变更记录

## 2026-07-03 · 初始建模（media_agent v1）
- 来自 `media_ai` 的 `Product` 设计迁入并按新权限模型重塑：
  - 旧 Product 字段 `userId` / `teamId` **移除**：用户明确"product 没有 user 概念"。product 不走资源级 ACL（无 owner / visibility / grants）。
  - 旧 Product 的业务字段（targetAudience / productDetails / displayActions / tags / isSellable / category 等）统一收进 `metadata` jsonb，避免 schema 频繁加列。
  - 新增 `created_by` / `updated_by` 仅作审计，不作 ACL。
- 可达性：product 绑定的 asset 在执行时走 AssetReachability 的 product 路径（仅 scope 匹配，无 user 校验），对应原需求"scope 一样就能用 product 的素材"。

### 2026-07-03 · Review 反馈：product 可作为 workflow 输入
- 纠正前期"product 不直接参与 workflow 执行"的表述：product **可以**作为 `workflow_input(type=product)`，调用方在 SubmitJob 时传 `product_id`。
- 执行时按目标 slot 的 `asset_kind` 从 product 绑定的 asset 中解析出唯一 asset 参与渲染（`ResolveAssetByKind`）。
- product 仍**不能**作为 step_binding 的直接 source——step_binding source 仍是 `workflow_input / step_output / const`。
- product 元数据不直接进 prompt（slot 只接 asset）。

## 修改记录

### 2026-07-21 · 跨域 RPC 上提 + 绑定端点收口
- `CheckProductAccessible` 从本 domain 能力清单移除，上提到 [reachability workflow](../../workflow/0001-reachability/reachability.md)（见 [ADR 0006](../../../adr/0006-reachability-layering.md)）。
- 移除 `AddAssetBinding` / `RemoveAssetBinding` 端点：product↔material 绑定收口到 [asset](../0002-asset/asset.md) domain 的 `AddMaterialBinding`（`owner_kind=product`），消除原"AddAssetBinding vs AddMaterialBinding"双头归属与命名漂移（asset binding / material binding）。
- `dependencies` 去掉 `scope`（已降为 shared）；`related_workflow` 补 reachability + media_workflow.validation/execution。
- 校验细则 5 补 `bindings_end_at` 置位（对齐 asset domain 声明）。

## 来自业务规则的强约束
1. product 无 user 归属；CRUD 不做 user 级权限校验（任意已认证 user 可操作，与"创建不限 scope"立场一致）。
2. product 绑定 asset 强制同 scope。
3. product 可作 workflow_input（type=product），但不能作 step_binding source。slot.asset_kind 充当解析选择器；解析到 0 或 >1 个 asset → 报错。
4. product 元数据不直接进 prompt——slot 只接 asset。
5. scope 不可改。
