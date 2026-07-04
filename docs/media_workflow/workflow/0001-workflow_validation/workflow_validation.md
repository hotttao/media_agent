---
name: workflow_validation
description: 把候选 Workflow 跨域校验一遍——DAG 结构、Template 状态、资产可达性、scope 一致性
dependencies:
    - workflow
    - template
    - materials: media_materials.AssetReachability
---

## 一句话职责
对一次 Create/Update/Submit 提交做"实际落库前的安全闸"：依次完成 DAG 结构校验 → Template 状态校验 → 资产可达性与 scope 校验。任一环节失败 → 给调用方返回 reject 原因 + 错误码**全局一致**，不写库。

## 触发点

- `POST /api/v1/workflows`、`PUT /api/v1/workflows/{id}` 进入保存前
- `POST /api/v1/workflows/validate`（不保存）
- `POST /api/v1/jobs` 进入 Submit 阶段

## 阶段

### 1. DAG 结构校验
调用 [workflow](../../domain/0002-workflow/workflow.md) §校验细则 的全部不变式——`workflow.Validator.Validate(w, templates)`。失败直接返回。成功后输出 roots + derived_edges。

### 2. Template 状态与跨资源权限校验
对每个 step 关联的 `(template_id, template_version)`：
1. `Template` 当前 status 必须 ∈ {draft, published}。**archived → reject** `TEMPLATE_ARCHIVED`。
2. 同 workflow scope 下 `Template.Scope` 必须等于 `Workflow.Scope`（**严格相等**）。不同 → reject `SCOPE_MISMATCH`。
3. **跨资源权限一致性**：caller（user）对所引 Template 必须 `my_permission ≥ viewer`（资源 ACL）；不足 → reject `TEMPLATE_INACCESSIBLE`。模板侧可视性由模板自己的 grants 决定，与 workflow.grants 独立。

> 这一阶段阻止"draft 模板被 workflow bind 后被 archive"的情况：用户 archive 一个 template 时，本 service 需异步扫一遍 workflow / job 中活跃 binding；本期先实现"保存时校验"，后续引入 offline scan。

### 3. 输入可达性 / scope 一致性校验
按 workflow_input.`type` 分派校验（锚点均为 `workflow.scope`，user 不持 scope）：
- **type=material**：调 `media_materials.AssetReachability(userID, workflowScope, [(MATERIAL, material_id)])`；material 不可达（scope 不匹配 / visibility 不通过 / 容器 vip 不可见或 product 不匹配）→ reject `ASSET_UNREACHABLE`。
- **type=asset**：调 `media_materials.AssetReachability(userID, workflowScope, [(ASSET, generated_asset_id)])`；generated_asset 不可达 → reject `ASSET_UNREACHABLE`。
- **type=product**：调 `media_materials.CheckProductAccessible(workflowScope, product_id)`；仅校验 `product.scope == workflowScope`（product 无 user 概念）。不匹配 → reject `SCOPE_MISMATCH`。
- **type=virtual_ip**：调 `media_materials.CheckVirtualIPAccessible(userID, workflowScope, virtual_ip_id)`；校验 `virtual_ip.scope == workflowScope` ∧ `MyPermission(user, vip) ≥ viewer`。不匹配 → reject `VIP_UNREACHABLE`。

slot.`kind` 合法性（kind 已注册、未停用、resource_type 与 workflow_input.type 自洽）由 `media_materials.BatchGetAssetKind` 在阶段 2 一并校验；对 product / virtual_ip 输入，slot.kind 的解析在 execution 阶段校验（见 [workflow_execution](../0002-workflow_execution/workflow_execution.md) §解析）。

### 4. Result
```go
type ValidationResult struct {
    Valid         bool
    Roots         []string
    OutputStepKey string
    DerivedEdges  []Edge
    RejectReasons []RejectReason
}
type RejectReason struct {
    Code    string // 'SCOPE_MISMATCH','TEMPLATE_ARCHIVED','ASSET_UNREACHABLE','DAG_INVALID',...
    Message string
    Subject string // template_id / asset_id / step_key 等
}
```

## 错误码（全局一致）

| Code | 触发场景 | HTTP 状态 |
|:---|:---|:---|
| `DAG_INVALID` | step key 重复 / 多 sink / 离群节点 / cycle | 400 |
| `TEMPLATE_NOT_FOUND` | step 引用 template 不存在 | 400 |
| `TEMPLATE_ARCHIVED` | 已 archive | 409 |
| `TEMPLATE_INACCESSIBLE` | caller 对引用的 template 无 viewer+ 权限 | 403 |
| `SCOPE_MISMATCH` | workflow.scope != template.scope 或 product.scope != workflow.scope | 409 |
| `ASSET_UNREACHABLE` | asset_id 不在调用方可达集合 | 403 |
| `VIP_UNREACHABLE` | virtual_ip 输入 scope 不匹配或 user 无 viewer+ | 403 |
| `ASSET_KIND_MISMATCH` | input slot.kind 与所传 asset 的 asset_kind 不一致 | 400 |
| `SLOT_KIND_REQUIRED` | slot 未声明 kind，或 kind.resource_type 与 workflow_input.type 不自洽 | 400 |
| `ASSET_KIND_INVALID` | slot.kind / output_asset_kind 未注册或已 DISABLED | 400 |
| `BINDING_INVALID` | to_input 不在 template.inputs.name 内 / required slot 未 binding | 400 |

## 失败处理
任何 RejectReason → workflow / job 不写库；调用方拿到的是 ValidationResult 整体，可视化展示给前端。

## 与 media_materials 契约
- RPC：`BatchGetAssetForValidation(ctx, caller{userID, workflowScope}, assetIDs []int64) → (assets []*Asset, missing []int64)`；`workflowScope` 是当前 workflow 的 scope，作可达性锚点。
- RPC：`BatchGetAssetKind(ctx, names []string) → (kinds []*AssetKind, invalid []string)`——读 `media_materials.asset_kinds` 注册表（全局，无 scope），返回每个 kind 的 resource_type / media_type / status。
- 不接受 media_materials 跨库直读。

## 关联 domain
- [workflow](../../domain/0002-workflow/workflow.md)
- [template](../../domain/0001-template/template.md)
- [execution](../../domain/0003-execution/execution.md)（Submit 阶段复用）

## 关联术语
[Workflow Validation](../../CONTEXT.md)、[Asset Reachability](../../media_materials/CONTEXT.md)。
