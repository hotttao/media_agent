---
name: virtual_ip
description: VirtualIp（虚拟形象）的存储、资源权限与关联 Asset / Product
dependencies:
    - asset_kind
related_workflow:
    - reachability
    - media_workflow.workflow_validation
---

## 一句话职责
virtual_ip domain 管理可写、可共享的 AI 形象，归属一个 Workspace、Scope 和 owner；带 visibility + grants，按 [My Permission](../../../resource_access/CONTEXT.md#my-permission有效资源权限) 推导。对 [reachability workflow](../../workflow/0001-reachability/reachability.md) 暴露可见性只读查询。

### 不做什么
- 不存 IP 形象二进制（基础图 avatarUrl / fullBodyUrl / threeViewUrl / nineViewUrl 等仅作 URL 字段存储；产物落 media_materials.Asset，IP 表只持引用）。
- 不执行 prompt / workflow / job（归 `media_workflow`）。
- 不做跨域可达性决策（`CheckVirtualIPAccessible` / `ResolveVirtualIpForReachability` 归 [reachability workflow](../../workflow/0001-reachability/reachability.md)，见 [ADR 0006](../../../adr/0006-reachability-layering.md)）；本 domain 仅暴露可见性只读查询供其编排。
- 不参与 publication_plan（待后续建模）。

## 边界

### 做什么
- VirtualIp 增删改查（含基础图片 URL 元数据）；强制 1:1 scope 且 scope 不可改（service 调 `shared/scope.IsKnownScope`）。
- VirtualIp 的资源权限（visibility + grants + derived my_permission）遵循 [共享资源访问语言](../../../resource_access/CONTEXT.md)。
- VirtualIp 名单可见性查询（`ListVirtualIpsVisibleTo(userID)`）。
- 对 reachability workflow 暴露 vip 可见性只读查询（供 `ResolveVirtualIpForReachability` / `CheckVirtualIPAccessible` 编排）。

### 不做什么
- 不管理 material / generated_asset（仅持 material_bindings 关系；material 归 [asset](../0002-asset/asset.md)，generated_asset 通过其 source_virtual_ip_id 反向关联）。
- 不管理 product（product 与 vip 平级，无直接关联）。

## 能力清单

| 能力 | 说明 | 对外接口 |
|:---|:---|:---|
| CreateVirtualIp | 新建 VirtualIp；scope + owner_user_id + visibility；owner=creator | `POST /api/v1/virtual-ips` |
| UpdateVirtualIp | 改基础字段 / 基础图片 URL；editor+ | `PUT /api/v1/virtual-ips/{id}` |
| GetVirtualIp | viewer+ | `GET /api/v1/virtual-ips/{id}` |
| ListVirtualIpsVisibleTo | 返回 user 可见的全部 VirtualIp（含 owner/grants/public） | `GET /api/v1/virtual-ips?scope=` |
| ArchiveVirtualIp | editor+ | `POST /api/v1/virtual-ips/{id}/archive` |
| SoftDeleteVirtualIp | owner only | `DELETE /api/v1/virtual-ips/{id}` |
| GrantVirtualIp | 写一条 virtual_ip_grants；owner | `POST /api/v1/virtual-ips/{id}/grants` |
| RevokeVirtualIp | owner | `DELETE /api/v1/virtual-ips/{id}/grants/{team_id}` |
| ChangeVisibility | owner | `POST /api/v1/virtual-ips/{id}/visibility` |

> 跨域 RPC `CheckVirtualIPAccessible` / `ResolveVirtualIpForReachability` 归 [reachability workflow](../../workflow/0001-reachability/reachability.md)，不在本 domain 能力清单。

## 数据模型

### Tables - `virtual_ips`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | bigserial | 主键 |
| scope | text not null | 业务范围；1:1；不可改 |
| name | text not null | IP 名 |
| description | text nullable | 说明 |
| avatar_url / full_body_url / three_view_url / nine_view_url | text nullable | 基础图 URL；IP 唯一身份信息 |
| metadata | jsonb default '{}' | 扩展字段 |
| status | text not null | active / archived；default active |
| visibility | text not null | private / team / public；default private |
| owner_user_id | text not null | 拥有者 |
| created_at / updated_at / deleted_at | timestamptz | 时间戳 |

约束：UNIQUE `(scope, name)` WHERE `deleted_at IS NULL`；INDEX `(scope, status)`；INDEX `(owner_user_id)`。

### Tables - `virtual_ip_grants`

`(virtual_ip_id, team_id) -> permission`，与 [Resource Grant](../../../resource_access/CONTEXT.md#resource-grant资源授权) 语义一致。

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| virtual_ip_id | bigint not null | FK；复合 PK；ON DELETE CASCADE |
| team_id | text not null | 团队 id；复合 PK |
| permission | text not null | viewer / editor；CHECK in ('viewer','editor') |
| granted_by | text not null | 授权者；仅 owner 可写 |
| granted_at | timestamptz | 时间戳 |

约束：PK `(virtual_ip_id, team_id)`；INDEX `(team_id)`。

## 权限规则

按 [My Permission](../../../resource_access/CONTEXT.md#my-permission有效资源权限) 推导：

| 操作 | 所需 my_permission |
|:---|:---|
| `CreateVirtualIp` | 无；创建者 = owner |
| `GetVirtualIp` / `ListVirtualIpsVisibleTo` | ≥ viewer |
| `UpdateVirtualIp`（基础字段 / 图片 URL / metadata） | ≥ editor |
| `ArchiveVirtualIp` | ≥ editor |
| `SoftDeleteVirtualIp` | owner |
| `GrantVirtualIp` / `RevokeVirtualIp` | owner |
| `ChangeVisibility` | owner |

## 作为 workflow 输入

virtual_ip 可作为 `workflow_input(type=virtual_ip)`，调用方在 SubmitJob 时传 `virtual_ip_id`：
- **校验**（Submit 阶段）：reachability workflow 的 `CheckVirtualIPAccessible(userID, workflowScope, virtual_ip_id)` 校验 `virtual_ip.scope == workflowScope` ∧ `MyPermission(user, vip) ≥ viewer`（资源 ACL）。
- **解析**（Execution 阶段）：当某 step 的 binding 引用该 workflow_input 时，按目标 slot 的 `kind` 调 reachability workflow 的 `ResolveAssetByKind(owner_kind=virtual_ip, owner_id=virtual_ip_id, asset_kind=slot.kind)`；按 slot.kind.resource_type 分发到 materials（material_bindings）或 generated_assets（source_virtual_ip_id）取唯一 asset；0 或 >1 个 -> execution 报错。
- virtual_ip 基础图（avatar_url / full_body_url 等）是 IP 身份字段，**不作为 asset 进 prompt**--slot 只接 asset；若 prompt 需基础图，需先上传为 asset 并绑定到 vip。
- virtual_ip **不能**作为 step_binding 的直接 source--必须先声明为 workflow_input，再用 `source=workflow_input` 引用。

## 校验细则

1. scope ∈ SupportedScopes（service 调 `shared/scope.IsKnownScope`）；scope 创建后不可改。
2. status=archived 不允许改回 active（archived 终态，与 template / workflow 的 archived 一致；vip 无 published 档）。
3. SoftDelete 时把 `material_bindings` 中 `owner_kind=virtual_ip` 的关联置 `bindings_end_at = now()`（不删，绑定失效可查；列定义见 [asset](../0002-asset/asset.md) §material_bindings）；`generated_assets.source_virtual_ip_id` 不动（保留生成历史）。

## 关联关系

### 依赖的 domain
- [asset_kind](../0005-asset_kind/asset_kind.md) - 无直接依赖（vip 不持 asset_kind 字段，仅作为 workflow 输入时由 slot.kind 间接引用）

### 被依赖的 domain / workflow
- [asset](../0002-asset/asset.md) - material_bindings owner_kind=virtual_ip
- [reachability workflow](../../workflow/0001-reachability/reachability.md) - 提供 vip 可见性只读查询供 `ResolveVirtualIpForReachability` / `CheckVirtualIPAccessible` 编排
- [`media_workflow` workflow_validation](../../../media_workflow/workflow/0001-workflow_validation/workflow_validation.md) - SubmitJob 期间经 reachability 校验

### 关联术语
[Virtual IP](../../CONTEXT.md#virtual-ip虚拟形象)、[Asset Binding](../../CONTEXT.md)、[Asset Reachability](../../CONTEXT.md)、[Resource Visibility](../../../resource_access/CONTEXT.md#resource-visibility资源可见性)。
