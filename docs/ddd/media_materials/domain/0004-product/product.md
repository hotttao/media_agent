---
name: product
description: Product（业务产品）的存储、scope 级共享与 material 绑定
dependencies:
    - asset
    - asset_kind
related_workflow:
    - library_object_upload
    - reachability
    - media_workflow.workflow_validation
    - media_workflow.workflow_execution
---

## 一句话职责
product domain 管理**业务产品**及其有序 Product Image 集合。Product 持 `organization_id` + `scope` + 业务字段，**无 user 归属、无 visibility、无 grants**--是 Organization 成员在同 Scope 内共享的资源。通过 `material_bindings(owner_kind=product)` 绑定同 Organization、同 Scope 的 Material。Product 可作为 Workflow 顶层输入，执行时按 Template Slot 的 kind 解析具体 Material/Generated Asset。

### 不做什么
- **不直接作为 step_binding 的 source**：step 要用 product，必须在 workflow 顶层声明为 `workflow_input(type=product)`，再用 `source=workflow_input` 引用。step_binding source 仍是 `workflow_input / step_output / const` 三选一。
- 不做资源级 user ACL（无 owner / visibility / grants）；但 Organization Membership 校验始终先于 Product 操作。
- 不管理 material / generated_asset 本身（仅持 material_bindings 关系；material 归 [asset](../0002-asset/asset.md)，generated_asset 通过 source_product_id 反向关联）。
- 不做跨域可达性决策（`CheckProductAccessible` 归 [reachability workflow](../../workflow/0001-reachability/reachability.md)，见 [ADR 0006](../../../adr/0006-reachability-layering.md)）。
- 不把 Product Image 当成 Material；图片随 Product 聚合维护。
- 不在 CRUD 内上传或删除 Storage 对象；调用方先经 [Library Object Upload](../../../workflow/0008-library-object-upload/library_object_upload.md) 由 media_materials 取得 `object_path`，Product Image 只保存该逻辑引用。
- 不管理 publication_plan（待后续建模）。

## 边界

### 做什么
- Product 增删改查；Organization、Scope 创建后不可改。
- 维护稳定业务字段：target_audience / product_details / display_actions / tags；外部电商字段收口为 External Commerce Reference。
- 在 Product 聚合事务内维护 Product Image 的有序集合与唯一主图。
- 维护 status（active / archived）。
- 同 Organization、同 Scope 的 Material 构成 Product 可组合素材集合；显式 Binding 属于后续阶段。
- 暴露 `ListProducts` / `GetProduct` 给前端管理 UI 与 workflow 调用方挑选 asset。

### 不做什么
- 不签发任何 ACL 决策（product 自身无 user 概念；可达性由 [reachability workflow](../../workflow/0001-reachability/reachability.md) 的 `CheckProductAccessible` 处理）。
- 不存素材二进制；Product Image 与 Asset 都只保存 Storage `object_path`。

## 能力清单

| 能力 | 说明 | 对外接口 |
|:---|:---|:---|
| CreateProduct | 新建 Product 与初始图片集合；created_by=caller（审计） | `POST /api/v1/products` |
| UpdateProduct | 改业务字段，并可原子替换图片集合 | `PUT /api/v1/products/{id}` |
| AddProductImage / RemoveProductImage | 单独增删图片；仍在 Product 聚合事务内校验主图不变量 | `POST/DELETE /api/v1/products/{id}/images` |
| GetProduct | Organization 内同 Scope 可读 | `GET /api/v1/products/{id}` |
| ListProducts | 按 scope/name/target_audience/tags/status 过滤分页 | `GET /api/v1/products?scope=&name=&target_audience=&tag=&status=` |
| ArchiveProduct | 改为 archived | `POST /api/v1/products/{id}/archive` |
| SoftDeleteProduct | 软删 | `DELETE /api/v1/products/{id}` |

> product 无 Grant / Revoke / ChangeVisibility / AddAssetBinding / RemoveAssetBinding 接口--它不走资源级 ACL；product↔material 绑定收口到 [asset](../0002-asset/asset.md) domain 的 `AddMaterialBinding`（`owner_kind=product`）。
> 跨域 RPC `CheckProductAccessible` 归 [reachability workflow](../../workflow/0001-reachability/reachability.md)。

## 数据模型

### Tables - `products`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | bigserial | 主键 |
| organization_id | text not null | 所属 Organization；不可改 |
| scope | text not null | 业务范围；1:1；不可改；校验 `IsKnownScope`（service 调 `shared/scope`） |
| name | text not null | 产品名；Organization + Scope 内唯一 |
| description | text nullable | 说明 |
| target_audience | text nullable | mens / womens / kids 等受众分类 |
| product_details | text nullable | 产品详情 |
| display_actions | jsonb not null | 展示动作列表；default '[]' |
| tags | jsonb not null | 标签列表；default '[]' |
| external_commerce | jsonb not null | External Commerce Reference；promotion_id、external_title、category、price、platform_content_id 等可选字段 |
| metadata | jsonb not null | 尚未进入稳定通用语言的扩展字段；default '{}' |
| status | text not null | active / archived；default active |
| created_by | text not null | 创建者（审计）；**仅审计，不参与 ACL** |
| updated_by | text nullable | 最近修改者（审计） |
| created_at / updated_at / deleted_at | timestamptz | 时间戳 |

约束：UNIQUE `(organization_id, scope, name)` WHERE `deleted_at IS NULL`；INDEX `(organization_id, scope, status)`；INDEX `(created_by)`。

### Tables - `product_images`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | bigserial | Product Image 标识 |
| organization_id | text not null | 必须与 Product 一致 |
| product_id | bigint not null | Product 聚合根；ON DELETE CASCADE 仅用于物理清理 |
| object_path | text not null | Storage 对象路径，不保存完整 URL |
| is_main | boolean not null | 是否主图；默认 false |
| sort_order | int not null | 显示顺序；非负 |
| created_at / updated_at | timestamptz | 时间戳 |

约束：同一 Product 至多一条 `is_main=true` 的部分唯一索引；UNIQUE `(product_id, sort_order)`。更新图片集合时整体校验后一次事务提交；存在图片时建议有主图，但允许迁移中暂时没有主图。

### Asset Binding

Product ↔ Material 的显式 Binding 属于后续阶段；第一阶段只按 Organization + Scope 查询可组合 Material，不提供 Binding 写接口。Generated Asset 通过 `source_product_id` 保留生成历史。

## 可达性规则（与 reachability workflow 协同）

product 自身不做可达性决策；其绑定的 asset 在 workflow 执行时的可达性由 [reachability workflow](../../workflow/0001-reachability/reachability.md) 的 `AssetReachability` 处理：

```
asset 可被引用 ⟺ asset.organization_id == request.organization_id
              ∧ asset.scope == workflowScope
              ∧ asset.visibility 规则通过
              ∧ ( exists visible virtual_ip binding
                  OR exists product binding )   // product 路径：Organization Membership + scope，无资源级 user ACL
```

Product 路径“无资源级 user ACL”对应原需求“Product 没有 user 概念，Scope 一样就能用 Product 的素材”；它不表示跨 Organization 公开。由于 Binding 强制 Asset 与 Product 同 Organization、同 Scope，且请求锚定 Organization/Workflow Scope，Product Binding 自动满足容器边界。

### 作为 workflow 输入

product 可作为 `workflow_input(type=product)`，调用方在 SubmitJob 时传 `product_id`：
- **校验**（Submit 阶段）：reachability workflow 的 `CheckProductAccessible(principal, organization_id, workflowScope, product_id)` 校验 Organization Membership、`product.organization_id == organization_id` 与 `product.scope == workflowScope`，不做 Product 资源级 user ACL。
- **解析**（Execution 阶段）：当某 step 的 binding 引用该 workflow_input 时，按目标 slot 的 `kind` 调 reachability workflow 的 `ResolveAssetByKind(owner_kind=product, owner_id=product_id, asset_kind=slot.kind)`；按 slot.kind.resource_type 分发到 materials（material_bindings）或 generated_assets（source_product_id）取唯一 asset；0 或 >1 个 -> execution 报错。
- product 元数据（name / targetAudience 等）**不直接进 prompt**--slot 只接 asset；若 prompt 需要 product 信息，需先将该信息存为 asset（如 `asset_kind=product_brief`）绑定到 product 并作为 slot 输入。

## 校验细则

1. Principal 必须是目标 Organization 的 active member；Product 的 Organization、Scope 创建后不可改。
2. Product 无 owner / visibility / grants；通过 Organization Membership 校验后，不再按用户区别 Product CRUD 权限。
3. 第一阶段以 Organization + Scope 作为 Product 与 Material 的组合边界，不实现 Binding。
4. status=archived 不影响已绑定 material 的可达性（archive 仅是产品状态标记，material 仍可通过 binding 被引用）；如需阻止引用 archived product 的 material，由后续策略加。
5. SoftDelete 使 Product 及其 Product Image 不可见，不删除 Storage 对象、Generated Asset 或生成历史。

## 关联关系

### 依赖的 domain
- [asset](../0002-asset/asset.md) - material_bindings owner_kind=product；绑定 CRUD 收口于此
- [asset_kind](../0005-asset_kind/asset_kind.md) - 间接（作为 workflow 输入时由 slot.kind 引用）

### 被依赖的 domain / workflow
- [reachability workflow](../../workflow/0001-reachability/reachability.md) - `CheckProductAccessible` / product 路径可达性
- [`media_workflow` workflow_validation](../../../media_workflow/workflow/0001-workflow_validation/workflow_validation.md) - SubmitJob 期间经 reachability 校验
- [`media_workflow` workflow_execution](../../../media_workflow/workflow/0002-workflow_execution/workflow_execution.md) - 经 reachability 解析 product 输入

### 关联术语
[Product](../../CONTEXT.md#product产品)、[Scope](../../../resource_access/CONTEXT.md#scope业务范围)、[Asset Binding](../../CONTEXT.md)、[Asset Reachability](../../CONTEXT.md)、[Resource Type](../../CONTEXT.md)。
