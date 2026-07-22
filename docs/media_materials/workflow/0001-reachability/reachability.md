---
name: reachability
description: 供 caller(BFF)组装 JobInput bundle 时校验可达性 + 解析 asset 的跨域门面
dependencies:
    - asset
    - virtual_ip
    - product
    - asset_kind
---

## 一句话职责

media_materials 对 **caller（BFF / 编排层）** 暴露的跨域可达性门面：编排 asset / virtual_ip / product 三个 domain 的只读查询加载事实，调 `policy/reachability` 做纯决策（scope ∧ visibility ∧ 容器可达），返回 `ReachabilityResult` 或解析出的 asset。**caller 在 SubmitJob 前调本 workflow 校验可达性 + 组装自包含的 JobInput bundle（含 url/content）；media_workflow 不调本 workflow**（execution 用 bundle，见 [media_workflow spec](../../../spec/media_workflow/impl.md) §6）。归层决策见 [ADR 0006](../../../adr/0006-reachability-layering.md)。

## 边界

### 做什么
- 对 caller（BFF）暴露 `AssetReachability` / `CheckProductAccessible` / `CheckVirtualIPAccessible`（SubmitJob 前校验输入可达性）。
- 对 caller 暴露 `ResolveAssetByKind` / `AssetRead` / `ResolveVirtualIpForReachability`（组装 bundle 时解析 product / virtual_ip 输入 + 拿资源 url/content）。
- 编排 asset / virtual_ip / product 的**只读查询**加载事实（不写）。
- 调 `policy/reachability` 做纯决策，自身不持有决策逻辑。

### 不做什么
- 不持久化（纯查询 + 决策，无表）。
- 不自行加载 user / team（team 列表由 media_auth 经 context 传入，或调 media_auth internal endpoint；见 [media_auth §资源级 ACL](../../../media_auth/CONTEXT.md#资源级-acl-所有媒体资源通用)）。
- **不被 media_workflow 调用**--media_workflow 用 caller 组装的 bundle；仅 `BatchGetAssetKind` + `UpsertGeneratedAsset` 由 media_workflow 直接调 asset_kind / generated_asset domain，不经本 workflow。
- 不暴露给前端管理 UI（供 caller 编排层）。
- 不返回资源二进制（`AssetRead` 只返 url + metadata）。

## 能力清单（供 caller 调用的跨服务 RPC）

| 能力 | 说明 | 对外接口 |
|:---|:---|:---|
| AssetReachability | 批量校验 refs 在 (userID, workflowScope) 下是否可引用；按 resource_type 分派 | `AssetReachability(userID, workflowScope, refs [(resource_type, id)])` |
| CheckProductAccessible | 校验 product.scope == workflowScope（product 无 user 概念） | `CheckProductAccessible(workflowScope, product_id)` |
| CheckVirtualIPAccessible | 校验 vip.scope == workflowScope ∧ MyPermission(user, vip) ≥ viewer | `CheckVirtualIPAccessible(userID, workflowScope, vip_id)` |
| ResolveVirtualIpForReachability | 返回 user 对哪些 vip 至少 viewer | `ResolveVirtualIpForReachability(userID, vipIDs)` |
| ResolveAssetByKind | 按 (ownerKind, ownerID, assetKind) 取唯一 asset；按 resource_type 分派 | `ResolveAssetByKind(ownerKind, ownerID, assetKind)` |
| AssetRead | 按 (resource_type, id) 取 url + metadata | `AssetRead(resource_type, id)` |

> caller 用这些 RPC 组装 JobInput bundle：校验可达性（`AssetReachability` / `Check*Accessible`）-> `ResolveAssetByKind` 解析 product / virtual_ip 输入为具体 material / generated_asset -> `AssetRead` 拿 url/content 填入 bundle。组装好的 bundle 提交 `media_workflow.SubmitJob`，media_workflow 不再调本 workflow。

## 阶段

### 1. AssetReachability 复合规则

对 `(userID, workflowScope, refs)`，`refs = [(resource_type, id), ...]`。按 resource_type 分派加载事实 + 决策。

#### material 可达性
加载：asset service 查 material（scope / visibility / owner_user_id / team_id / bindings[virtual_ip, product]）。
决策（policy/reachability）：
```
can = (material.scope == workflowScope)
   ∧ visibility_rule(material, userID)
   ∧ ( exists binding owner_kind=virtual_ip 且 user 对该 vip 可见（MyPermission ≥ viewer）
       or exists binding owner_kind=product )   // product 路径：仅 scope 匹配，无 user
```

#### generated_asset 可达性
加载：asset service 查 generated_asset（scope / visibility / owner / team / source_virtual_ip_id / source_product_id）。
决策：
```
can = (generated_asset.scope == workflowScope)
   ∧ visibility_rule(generated_asset, userID)
   ∧ ( generated_asset.source_virtual_ip_id 对 user 可见
       or generated_asset.source_product_id 非空 )   // product 路径：仅 scope 匹配
```

#### product / virtual_ip 输入可达性
- product：`CheckProductAccessible(workflowScope, product_id)` - 仅 `product.scope == workflowScope`。
- virtual_ip：`CheckVirtualIPAccessible(userID, workflowScope, vip_id)` - `vip.scope == workflowScope` ∧ `MyPermission(user, vip) ≥ viewer`。

`visibility_rule(asset, user)`：
```
asset.visibility == 'public'
or (asset.visibility == 'team'    and asset.team_id in userTeams(user))
or (asset.visibility == 'private' and asset.owner_user_id == user)
```

返回 `ReachabilityResult{ Reachable map[ref]bool, Blocked []{Ref, Reason} }`，`Reason ∈ {SCOPE_MISMATCH, NOT_VISIBLE, CONTAINER_UNREACHABLE, NOT_FOUND}`。

### 2. ResolveAssetByKind

按 asset_kind.resource_type 分发（先调 asset_kind service 查 kind.resource_type）：
- `MATERIAL` -> 查 materials 表（经 `material_bindings` 找 owner 下的 material）。
- `ASSET` -> 查 generated_assets 表（经 `source_virtual_ip_id` / `source_product_id` 找 owner 下的 generated_asset）。

取 owner（virtual_ip / product）下 asset_kind 匹配的唯一 asset。`0` 个 -> `ErrAssetResolveMissing`；`>1` 个 -> `ErrAssetResolveAmbiguous`。

> product / virtual_ip 输入在 caller 组装 bundle 阶段调本 RPC：`ownerKind ∈ {product, virtual_ip}`，`assetKind = 目标 slot.kind`，按 slot.kind.resource_type 分发到 materials 或 generated_assets。

### 3. AssetRead

按 `(resource_type, id)` 分派到 materials / generated_assets 取 `url` + `metadata`。**不返回二进制**--素材文件由 storage 子系统按 url 提供，本 RPC 仅给 bundle 组装所需的定位与 hint。

## 关键流程参数传递

- `userID`：caller（BFF）从提交用户传入；reachability 不自行解析 user 身份。
- `workflowScope`：可达性锚点 = `workflow.scope`；所有 ref 的 scope 必须与之一致，否则 `SCOPE_MISMATCH`。
- `userTeams(user)`：由 media_auth 提供（fetchTeams 当前为 stub，team 可见性降级为 false--见 media_auth team 接入待办）。
- caller 组装 bundle 后，media_workflow 用 bundle 渲染，execution 阶段不再需要 userID/scope 做可达性（已由 caller 校验）。

## 幂等设计

全部为只读查询 + 纯决策，无副作用，天然幂等；重复调用结果一致（忽略并发写导致的中间态变化）。

## 关联 domain

- [asset](../../domain/0002-asset/asset.md) - 提供 material / generated_asset 只读查询
- [virtual_ip](../../domain/0003-virtual_ip/virtual_ip.md) - 提供 vip 可见性查询
- [product](../../domain/0004-product/product.md) - 提供 product scope 查询
- [asset_kind](../../domain/0005-asset_kind/asset_kind.md) - 提供 resource_type 查询（ResolveAssetByKind 分派用）

## 关联术语

[Asset Reachability](../../CONTEXT.md)、[Asset](../../CONTEXT.md)、[Resource Type](../../CONTEXT.md)、[Visibility On Resource](../../../media_auth/CONTEXT.md)。
