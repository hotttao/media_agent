---
name: asset
description: material（curated 原料）+ generated_asset（workflow 产物）两张表 + 跨域 AssetReachability
dependencies:
    - scope
    - asset_kind
    - docs/research/permission.md
related_workflow: []
---

## 一句话职责
asset domain 管理**两类物理资源**，分落两张表：
- **material**（`materials` 表，resource_type=MATERIAL）：curated 原料（scene/pose/makeup/movement/music/caption），用户上传/策展，**无生成记录**。
- **generated_asset**（`generated_assets` 表，resource_type=ASSET）：workflow 执行产物（model_image/style_image/first_frame/ai_video），**持生成记录**（job_id / step_run_id / template_id / template_version）。

两者都引用 [asset_kind](../0005-asset_kind/asset_kind.md) 注册表（`asset_kind` 字段 → `asset_kinds.name`）；都持 scope / owner / visibility / team_id；可见性沿用 [资源级 ACL](../../media_auth/CONTEXT.md#资源级-acl-所有媒体资源通用) 的 `private/team/public` 三档（无 grants 表）。暴露 `AssetReachability` 给 `media_workflow`。

### 不做什么
- 不存素材二进制（用 `url` 引用，文件由 storage 子系统提供）。
- 不执行 workflow / job / render（归 `media_workflow`）。
- 不把 material 与 generated_asset 混在一张表——generated_asset 必须记录"怎么生成的"，material 不需要，字段与生命周期不同。

## 边界

### 做什么
- material 增删改查 + `material_bindings`（material ↔ virtual_ip / material ↔ product，N:N）。
- generated_asset 增（由 workflow_execution upsert）/ 查 / 软删；持生成记录链接。
- 提供 `AssetReachability(userID, workflowScope, refs)` 给 `media_workflow.workflow_validation`——`refs` 是 `(resource_type, id)` 列表，按 resource_type 分派到 materials / generated_assets / products / virtual_ips 表。
- 提供 `ResolveAssetByKind(owner_kind, owner_id, asset_kind)` —— 按 asset_kind.resource_type 分发到 materials 或 generated_assets 表，取唯一 asset。
- 提供 `AssetRead(resource_type, id) → (url, metadata)` 给 `media_workflow.workflow_execution`。

### 不做什么
- 不做 user 级创建授权（创建不限 scope，见 [Scope](../../media_auth/CONTEXT.md#scope业务范围)）。
- 不管理 publication_plan。

## 能力清单

| 能力 | 说明 | 对外接口 |
|:---|:---|:---|
| UpsertMaterial | 新建/更新 material；asset_kind.resource_type 必须=MATERIAL | `POST /api/v1/materials` `PUT /api/v1/materials/{id}` |
| ListMaterials | 按 scope/asset_kind/owner/visibility 过滤 | `GET /api/v1/materials?scope=&asset_kind=&owner=&visibility=` |
| SoftDeleteMaterial | 软删 | `DELETE /api/v1/materials/{id}` |
| AddMaterialBinding | material ↔ vip / material ↔ product；两端同 scope | `POST /api/v1/materials/{id}/bindings` |
| GetGeneratedAsset | 按 id 取（含生成记录链接） | `GET /api/v1/generated-assets/{id}` |
| ListGeneratedAssets | 按 scope/asset_kind/owner/job_id 过滤 | `GET /api/v1/generated-assets?scope=&asset_kind=&job_id=` |
| SoftDeleteGeneratedAsset | 软删 | `DELETE /api/v1/generated-assets/{id}` |
| AssetReachability | 跨域 RPC：复合校验 scope ∧ visibility ∧ 容器可达 | `AssetReachability` |
| ResolveAssetByKind | 跨域 RPC：按 (owner_kind, owner_id, asset_kind) 取唯一 asset | `ResolveAssetByKind` |
| AssetRead | 跨域 RPC：按 (resource_type, id) 拿 url + metadata | `AssetRead` |

> generated_asset 的创建由 `media_workflow.workflow_execution` 在 step 产出时调 `UpsertGeneratedAsset` RPC 完成，不暴露 HTTP 给前端直接创建。

## 数据模型

### Tables — `materials`

| 字段 | 描述 | 类型 | 备注 |
|:---|:---|:---|:---|
| id | 主键 | bigserial | — |
| scope | 业务范围 | text not null | 1:1；校验 `IsKnownScope` |
| asset_kind | 素材类别 | text not null | 引用 `asset_kinds.name`，且其 resource_type=MATERIAL；校验已注册未停用 |
| name | 素材名 | text not null | (scope, asset_kind, name) 唯一 |
| url | 文件位置 | text not null | — |
| metadata | 渲染 hint | jsonb | default '{}' |
| owner_user_id | 创建者 | text not null | — |
| visibility | 可见性 | text not null | private / team / public；默认 private |
| team_id | 可见 team | text nullable | 仅 visibility=team 时使用 |
| created_at / updated_at / deleted_at | 时间戳 | timestamptz | — |

约束：UNIQUE `(scope, asset_kind, name)` WHERE `deleted_at IS NULL`；INDEX `(scope, asset_kind, updated_at desc)`；INDEX `(owner_user_id)`。

### Tables — `generated_assets`

| 字段 | 描述 | 类型 | 备注 |
|:---|:---|:---|:---|
| id | 主键 | bigserial | — |
| scope | 业务范围 | text not null | 沿用 job.workflow.scope |
| asset_kind | 产物类别 | text not null | 引用 `asset_kinds.name`，且其 resource_type=ASSET；= 产出该 asset 的 template.output_asset_kind |
| name | 产物名 | text not null | (scope, asset_kind, name) 唯一 |
| url | 文件位置 | text not null | — |
| metadata | 渲染 hint | jsonb | default '{}' |
| owner_user_id | 提交者 | text not null | = job.submitted_by |
| visibility | 可见性 | text not null | 默认 private（产物默认仅提交者可见） |
| team_id | 可见 team | text nullable | — |
| job_id | 生成 job | bigint not null | FK → media_workflow.jobs.id |
| step_run_id | 生成 step | bigint not null | FK → media_workflow.step_runs.id；生成记录详情在 step_run |
| template_id | 产出模板 | bigint not null | — |
| template_version | 产出模板版本 | int not null | 不可变快照 |
| source_virtual_ip_id | 源虚拟形象 | bigint nullable | job 输入里的 virtual_ip（若有），用于可达性 |
| source_product_id | 源产品 | bigint nullable | job 输入里的 product（若有） |
| created_at / updated_at / deleted_at | 时间戳 | timestamptz | — |

约束：UNIQUE `(scope, asset_kind, name)` WHERE `deleted_at IS NULL`；INDEX `(scope, asset_kind, created_at desc)`；INDEX `(job_id)`；INDEX `(source_virtual_ip_id)`；INDEX `(source_product_id)`。

> **生成记录**：`generated_asset` 持 `job_id` + `step_run_id` 反向链接到 execution 域的 StepRun（StepRun 已存 resolved_inputs / rendered_prompt / template_version 等完整生成上下文）。不另建 generation_records 表，避免冗余。

### Tables — `material_bindings`

material ↔ virtual_ip / material ↔ product 的 N:N 绑定。

| 字段 | 描述 | 类型 | 备注 |
|:---|:---|:---|:---|
| material_id | FK materials | bigint not null | 复合 PK 一段 |
| owner_kind | virtual_ip / product | text not null | 复合 PK 二段 |
| owner_id | vip_id 或 product_id | bigint not null | 复合 PK 三段 |
| bound_by | 谁绑定 | text not null | — |
| bound_at | 时间戳 | timestamptz | — |

约束：PK `(material_id, owner_kind, owner_id)`；INDEX `(owner_kind, owner_id)`。INSERT 前 service 校验 `IsSameScope(material.scope, owner.scope)`。

> generated_asset 不走 material_bindings——它的"容器"是生成记录里的 `source_virtual_ip_id` / `source_product_id`，由 execution 在 upsert 时填入。

## AssetReachability 复合规则

对 `(userID, workflowScope, refs)`，其中 `refs = [(resource_type, id), ...]`。按 resource_type 分派：

### material 可达性
```
can = (material.scope == workflowScope)
   ∧ visibility_rule(material, userID)
   ∧ exists material_binding owner_kind=virtual_ip 且 user 对该 vip 可见（MyPermission ≥ viewer）
     or exists material_binding owner_kind=product   // product 路径：仅 scope 匹配，无 user 校验
```

### generated_asset 可达性
```
can = (generated_asset.scope == workflowScope)
   ∧ visibility_rule(generated_asset, userID)
   ∧ ( generated_asset.source_virtual_ip_id 对 user 可见
       or generated_asset.source_product_id 非空 )   // product 路径：仅 scope 匹配
```

### product / virtual_ip 输入可达性
- product：`CheckProductAccessible(workflowScope, product_id)` — 仅 scope 匹配。
- virtual_ip：`CheckVirtualIPAccessible(userID, workflowScope, vip_id)` — scope 匹配 ∧ MyPermission ≥ viewer。

`visibility_rule(asset, user)`：
```
asset.visibility == 'public'
or (asset.visibility == 'team'    and asset.team_id in userTeams(user))
or (asset.visibility == 'private' and asset.owner_user_id == user)
```

返回 `ReachabilityResult{ Reachable map, Blocked []{Ref, Reason} }`，Reason ∈ {SCOPE_MISMATCH, NOT_VISIBLE, CONTAINER_UNREACHABLE, NOT_FOUND}。

## ResolveAssetByKind

```go
// 按 asset_kind.resource_type 分发：
// - MATERIAL → 查 materials 表
// - ASSET    → 查 generated_assets 表
// 取 owner (virtual_ip/product) 下 asset_kind 匹配的唯一 asset。
// 0 个 → ErrAssetResolveMissing；>1 个 → ErrAssetResolveAmbiguous。
func ResolveAssetByKind(ctx, ownerKind, ownerID, assetKind string) (assetID int64, err error)
```

material 走 `material_bindings` 找 owner 下的 material；generated_asset 走 `source_virtual_ip_id` / `source_product_id` 找 owner 下的 generated_asset。

## 校验细则

1. material.asset_kind / generated_asset.asset_kind 必须 ∈ asset_kinds 注册表且 status∈{SYSTEM, ENABLED}；resource_type 与表一致（material→MATERIAL，generated_asset→ASSET）。
2. 创建后 scope 不可改；asset_kind 不可改。
3. material_bindings 强制两端同 scope。
4. visibility=team 时 team_id 必填。
5. generated_asset 的 job_id / step_run_id / template_id / template_version 在 upsert 时由 execution 填入，不可改。
6. SoftDelete 不级联删 material_bindings（保留历史）；可达性查询忽略已软删记录。

## 关联关系

### 被依赖的 domain
- [`media_workflow.workflow_validation`](../../../media_workflow/workflow/0001-workflow_validation/workflow_validation.md) — AssetReachability / Check*Accessible
- [`media_workflow.workflow_execution`](../../../media_workflow/workflow/0002-workflow_execution/workflow_execution.md) — ResolveAssetByKind / AssetRead / UpsertGeneratedAsset
- [`media_workflow.execution`](../../../media_workflow/domain/0003-execution/execution.md) — generated_asset 反向链接 job_id / step_run_id

### 关联术语
[Material](../../CONTEXT.md)、[Generated Asset](../../CONTEXT.md)、[Asset Kind](../../CONTEXT.md)、[Asset Binding](../../CONTEXT.md)、[Asset Reachability](../../CONTEXT.md)。
