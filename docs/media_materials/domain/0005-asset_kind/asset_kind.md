---
name: asset_kind
description: asset_kind 注册实体——全平台资源类别的统一注册表（name → resource_type / media_type / source_table）
dependencies: []
related_workflow:
    - media_workflow.workflow_validation
---

## 一句话职责
asset_kind domain 管理**全平台资源类别的注册表**：每条 `asset_kinds` 记录声明一个资源类别（如 `scene` / `pose` / `model_image` / `product_image`），承载 `resource_type`（落哪张物理表）、`media_type`（媒体形态）、`source_table`、`status`。Template 的 input slot `kind` 与 output `asset_kind` 都引用此处的 `name`。**全局注册，不绑 scope**——`scene` 在所有业务线里是同一个含义。

### 不做什么
- 不绑 scope（asset_kind 是全局类型，不是业务线属性；业务线归属由 asset / product / virtual_ip / template / workflow 自身的 scope 字段承担）。
- 不存资源实例（只存"类别"；实例归 [asset](../0002-asset/asset.md) / [product](../0004-product/product.md) / [virtual_ip](../0003-virtual_ip/virtual_ip.md)）。
- 不做渲染 / 不参与 workflow 执行编排（归 `media_workflow`）。

## 边界

### 做什么
- asset_kind CRUD：Register / Get / List / Update / Delete。
- 维护 `resource_type ∈ {MATERIAL, ASSET, PRODUCT, VIRTUAL_IP, TEXT}`——标识该 kind 的物理资源类别（落哪张表）。
- 维护 `media_type ∈ {IMAGE, TEXT, VIDEO}`——媒体形态，Template 渲染与下游生成器分发依赖此值。
- 维护 `status ∈ {SYSTEM, ENABLED, DISABLED}`：SYSTEM 系统预置不可删/不可改 resource_type；DISABLED 软停用（列表仍展示但 Template 创建/校验拒绝引用）。
- 校验：Template 创建/更新时，input slot `kind` 与 output `asset_kind` 必须指向已注册且 status∈{SYSTEM, ENABLED} 的 asset_kind。

### 不做什么
- 不校验 scope（asset_kind 无 scope 字段）。
- 不解析 binding（归 `media_workflow.workflow_binding`）。

## 能力清单

| 能力 | 说明 | 对外接口 |
|:---|:---|:---|
| RegisterAssetKind | 注册新 kind；name 全局唯一 | `POST /api/v1/asset-kinds` |
| GetAssetKind | 按 name 取 | `GET /api/v1/asset-kinds/{name}` |
| ListAssetKinds | 按 resource_type / media_type / status 过滤 | `GET /api/v1/asset-kinds?resource_type=&media_type=&status=` |
| UpdateAssetKind | 改 description / status；SYSTEM 锁 resource_type/media_type/source_table | `PUT /api/v1/asset-kinds/{name}` |
| DeleteAssetKind | 仅 ENABLED 可删；SYSTEM 不可删 | `DELETE /api/v1/asset-kinds/{name}` |
| BatchGetAssetKind | 跨域 RPC：批量校验 kind 名合法且未停用 | `BatchGetAssetKind` |

## 数据模型

### Tables — `asset_kinds`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | bigserial | 主键 |
| name | text not null | 资源名（业务主键）；全局唯一；如 `pose`/`scene`/`model_image`/`product_image` |
| resource_type | text not null | 物理资源类别；CHECK in ('MATERIAL','ASSET','PRODUCT','VIRTUAL_IP','TEXT') |
| media_type | text not null | 媒体形态；CHECK in ('IMAGE','TEXT','VIDEO')；锁定不可改。注意与 resource_type 的 `TEXT`（无表 inline 常量）**正交**：`media_type=TEXT` 指内容形态，`resource_type=TEXT` 指不落表（见 [CONTEXT §Media Type](../../CONTEXT.md)） |
| source_table | text not null | 落表名；见下表；锁定不可改 |
| description | text nullable | 业务描述 |
| status | text not null | 状态；CHECK in ('SYSTEM','ENABLED','DISABLED')；default ENABLED |
| created_at / updated_at | timestamptz | 时间戳 |

约束：UNIQUE `(name)`；INDEX `(resource_type, status)`；INDEX `(media_type)`。

### resource_type → source_table 映射

| resource_type | 物理表 | 含义 |
|:---|:---|:---|
| MATERIAL | `materials` | curated 原料（scene/pose/makeup/movement/music） |
| ASSET | `generated_assets` | workflow 产物（model_image/style_image/first_frame/ai_video），持生成记录 |
| PRODUCT | `products` | 业务产品 |
| VIRTUAL_IP | `virtual_ips` | 虚拟形象 |
| TEXT | 无表（inline） | 字面常量 |

> material 与 generated_asset 分两张表，因 generated_asset 须记录生成记录（job/step/template），material 无此需求（见 [asset](../0002-asset/asset.md)）。

## 校验细则

1. `name` 全局唯一；创建后不可改（改 name 等于新 kind，避免悬挂引用）。
2. `resource_type` / `media_type` / `source_table` 创建后锁定不可改（SYSTEM 与 ENABLED 都锁；改这些等于改变 kind 语义）。
3. SYSTEM 状态：不可 Delete，不可改 `resource_type` / `media_type` / `source_table` / `name`（仅可改 description / status↔DISABLED）。
4. DISABLED：ListAssetKinds 仍返回（标注停用），但 Template 创建/更新引用该 kind → reject `ASSET_KIND_DISABLED`。
5. Delete：仅 ENABLED 可删；已有 asset / template 引用该 kind 时 → reject `ASSET_KIND_IN_USE`（防悬挂）。
6. `resource_type=TEXT` 的 kind 仅用于 step_binding 的 `const` 源（字面常量，无表）；不可作为 Template 的 `output_asset_kind`（产物须落 `generated_assets` 表，resource_type=ASSET），亦不可作为 material / generated_asset 的 asset_kind。

## 与其它域的关系

- **[asset](../0002-asset/asset.md)**：`materials.asset_kind` / `generated_assets.asset_kind` 引用 `asset_kinds.name`；UpsertMaterial 时校验 kind 已注册、未停用、resource_type 与表一致（material→MATERIAL，generated_asset→ASSET）。asset 的 media_type 由其 asset_kind.media_type 推导（不另存）。
- **[media_workflow template](../../../media_workflow/domain/0001-template/template.md)**：input slot `kind` 与 output `asset_kind` 引用 `asset_kinds.name`；通过 `BatchGetAssetKind` RPC 校验。
- **[media_workflow workflow](../../../media_workflow/domain/0002-workflow/workflow.md)**：step_binding 的 slot.kind 校验同上。

## 关联术语
[Asset Kind](../../CONTEXT.md)（注册实体，全局）、[Asset](../../CONTEXT.md)、[Resource Type](../../CONTEXT.md)、[Media Type](../../CONTEXT.md)。
