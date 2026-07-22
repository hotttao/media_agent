---
name: product
description: Product（业务产品）的存储、scope 级共享与 material 绑定
dependencies:
    - asset
    - asset_kind
related_workflow:
    - reachability
    - media_workflow.workflow_validation
    - media_workflow.workflow_execution
---

## 一句话职责
product domain 管理**业务产品**：一个可被视频带货的产品。product 持 `scope` + 业务元数据，**无 user 归属、无 visibility、无 grants**--是 scope 级共享资源。通过 `material_bindings(owner_kind=product)` 绑定同 scope 的 material（绑定操作收口在 [asset](../0002-asset/asset.md) domain 的 `AddMaterialBinding`）。product 可作为 workflow 的顶层输入（`workflow_input.type=product`），执行时按所绑 template slot 的 `kind` 解析出具体 material/generated_asset 参与渲染（见 [media_workflow §workflow_execution](../../media_workflow/workflow/0002-workflow_execution/workflow_execution.md)）。

### 不做什么
- **不直接作为 step_binding 的 source**：step 要用 product，必须在 workflow 顶层声明为 `workflow_input(type=product)`，再用 `source=workflow_input` 引用。step_binding source 仍是 `workflow_input / step_output / const` 三选一。
- 不做 user 级访问控制（无 owner / visibility / grants）。
- 不管理 material / generated_asset 本身（仅持 material_bindings 关系；material 归 [asset](../0002-asset/asset.md)，generated_asset 通过 source_product_id 反向关联）。
- 不做跨域可达性决策（`CheckProductAccessible` 归 [reachability workflow](../../workflow/0001-reachability/reachability.md)，见 [ADR 0006](../../../adr/0006-reachability-layering.md)）。
- 不单独暴露 product↔material 绑定端点（收口到 asset domain 的 `AddMaterialBinding`，`owner_kind=product`）。
- 不管理 publication_plan（待后续建模）。

## 边界

### 做什么
- product 增删改查；强制 1:1 scope 且 scope 不可改（service 调 `shared/scope.IsKnownScope`）。
- 维护业务元数据：name / description / metadata（targetAudience / productDetails / displayActions / tags / isSellable / category 等扩展字段，统一放 metadata jsonb）。
- 维护 status（active / archived）。
- 通过 [asset](../0002-asset/asset.md) 域的 `material_bindings(owner_kind=product)` 把 product 与同 scope 的 material 关联（N:N）--绑定 CRUD 走 asset domain 的 `AddMaterialBinding` / `RemoveMaterialBinding`，不在本 domain 重复开端点。
- 暴露 `ListProducts` / `GetProduct` 给前端管理 UI 与 workflow 调用方挑选 asset。

### 不做什么
- 不签发任何 ACL 决策（product 自身无 user 概念；可达性由 [reachability workflow](../../workflow/0001-reachability/reachability.md) 的 `CheckProductAccessible` 处理）。
- 不存素材二进制（asset 才存 url）。

## 能力清单

| 能力 | 说明 | 对外接口 |
|:---|:---|:---|
| CreateProduct | 新建 product；body 带 scope（不限 user）；created_by=caller（审计） | `POST /api/v1/products` |
| UpdateProduct | 改 name/description/metadata/status；updated_by=caller（审计） | `PUT /api/v1/products/{id}` |
| GetProduct | 任意 user 可读 | `GET /api/v1/products/{id}` |
| ListProducts | 按 scope/name/status 过滤分页；任意 user 可读 | `GET /api/v1/products?scope=&name=&status=` |
| ArchiveProduct | 改为 archived | `POST /api/v1/products/{id}/archive` |
| SoftDeleteProduct | 软删 | `DELETE /api/v1/products/{id}` |

> product 无 Grant / Revoke / ChangeVisibility / AddAssetBinding / RemoveAssetBinding 接口--它不走资源级 ACL；product↔material 绑定收口到 [asset](../0002-asset/asset.md) domain 的 `AddMaterialBinding`（`owner_kind=product`）。
> 跨域 RPC `CheckProductAccessible` 归 [reachability workflow](../../workflow/0001-reachability/reachability.md)。

## 数据模型

### Tables - `products`

| 字段 | 描述 | 类型 | 备注 |
|:---|:---|:---|:---|
| id | 主键 | bigserial | - |
| scope | 业务范围 | text not null | 1:1；不可改；校验 `IsKnownScope`（service 调 `shared/scope`） |
| name | 产品名 | text not null | (scope, name) 唯一 |
| description | 说明 | text nullable | - |
| metadata | 业务元数据 | jsonb not null | default '{}'；含 targetAudience / productDetails / displayActions / tags / isSellable / category 等 |
| status | active / archived | text not null | default active |
| created_by | 创建者（审计） | text not null | **仅审计，不参与 ACL** |
| updated_by | 最近修改者（审计） | text nullable | - |
| created_at / updated_at / deleted_at | 时间戳 | timestamptz | - |

约束：UNIQUE `(scope, name)` WHERE `deleted_at IS NULL`；INDEX `(scope, status)`；INDEX `(created_by)`。

### Asset Binding

product ↔ material 复用 [asset](../0002-asset/asset.md) 域的 `material_bindings` 表，`owner_kind='product'`、`owner_id=products.id`。绑定 CRUD 走 asset domain 的 `AddMaterialBinding` / `RemoveMaterialBinding`（INSERT 前 service 校验 `IsSameScope(material.scope, product.scope)`）。generated_asset 不走此表--它通过 `source_product_id` 反向关联 product（由 execution 在 upsert 时填入）。

## 可达性规则（与 reachability workflow 协同）

product 自身不做可达性决策；其绑定的 asset 在 workflow 执行时的可达性由 [reachability workflow](../../workflow/0001-reachability/reachability.md) 的 `AssetReachability` 处理：

```
asset 可被引用 ⟺ asset.scope == workflowScope
              ∧ asset.visibility 规则通过
              ∧ ( exists visible virtual_ip binding
                  OR exists product binding )   // product 路径：仅 scope 匹配，无 user 校验
```

product 路径"无 user 校验"对应原需求"product 没有 user 概念，scope 一样就能用 product 的素材"。由于 `asset.scope == product.scope`（binding 时强制）且 `asset.scope == workflowScope`，product 绑定自动满足 scope 匹配。

### 作为 workflow 输入

product 可作为 `workflow_input(type=product)`，调用方在 SubmitJob 时传 `product_id`：
- **校验**（Submit 阶段）：reachability workflow 的 `CheckProductAccessible(workflowScope, product_id)` 仅校验 `product.scope == workflowScope`，无 user 校验。
- **解析**（Execution 阶段）：当某 step 的 binding 引用该 workflow_input 时，按目标 slot 的 `kind` 调 reachability workflow 的 `ResolveAssetByKind(owner_kind=product, owner_id=product_id, asset_kind=slot.kind)`；按 slot.kind.resource_type 分发到 materials（material_bindings）或 generated_assets（source_product_id）取唯一 asset；0 或 >1 个 -> execution 报错。
- product 元数据（name / targetAudience 等）**不直接进 prompt**--slot 只接 asset；若 prompt 需要 product 信息，需先将该信息存为 asset（如 `asset_kind=product_brief`）绑定到 product 并作为 slot 输入。

## 校验细则

1. product.scope 必须 ∈ SupportedScopes（service 调 `shared/scope.IsKnownScope`）；创建后不可改。
2. product 无 owner / visibility / grants 字段；CRUD 不做 user 级权限校验（任意已认证 user 可操作）。
3. material_bindings 强制 product 与 material 同 scope（asset domain 的 `AddMaterialBinding` 校验）。
4. status=archived 不影响已绑定 material 的可达性（archive 仅是产品状态标记，material 仍可通过 binding 被引用）；如需阻止引用 archived product 的 material，由后续策略加。
5. SoftDelete 不级联删 material_bindings，但置其 `bindings_end_at = now()`（列定义见 [asset](../0002-asset/asset.md) §material_bindings）；已软删 product 的 binding 在可达性查询中被过滤。

## 关联关系

### 依赖的 domain
- [asset](../0002-asset/asset.md) - material_bindings owner_kind=product；绑定 CRUD 收口于此
- [asset_kind](../0005-asset_kind/asset_kind.md) - 间接（作为 workflow 输入时由 slot.kind 引用）

### 被依赖的 domain / workflow
- [reachability workflow](../../workflow/0001-reachability/reachability.md) - `CheckProductAccessible` / product 路径可达性
- [`media_workflow` workflow_validation](../../media_workflow/workflow/0001-workflow_validation/workflow_validation.md) - SubmitJob 期间经 reachability 校验
- [`media_workflow` workflow_execution](../../media_workflow/workflow/0002-workflow_execution/workflow_execution.md) - 经 reachability 解析 product 输入

### 关联术语
[Product](../../media_auth/CONTEXT.md#product业务产品)、[Scope](../../media_auth/CONTEXT.md#scope业务范围)、[Asset Binding](../../CONTEXT.md)、[Asset Reachability](../../CONTEXT.md)、[Resource Type](../../CONTEXT.md)。
