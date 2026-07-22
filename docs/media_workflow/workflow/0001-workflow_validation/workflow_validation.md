---
name: workflow_validation
description: 把候选 Workflow 跨域校验一遍——DAG 结构、Template 状态、资产可达性、scope 一致性
dependencies:
    - workflow
    - template
    - media_materials.asset_kind
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

### 3. 输入可达性校验（职责在 caller，不在 media_workflow）
media_workflow 采用 bundle 模式（见 [media_workflow spec](../../spec/media_workflow/impl.md) §6）：caller（BFF）在 SubmitJob 前调 [media_materials reachability workflow](../../../media_materials/workflow/0001-reachability/reachability.md) 校验输入可达性 + 组装 JobInput bundle（含 url/content）。media_workflow 不做可达性校验，不调 reachability。

- **workflow 保存时（闸 1）**：只校验 slot.kind 注册（`BatchGetAssetKind`），不校验 asset 可达性（workflow 是定义，无 asset 实例）。
- **SubmitJob 时（闸 2）**：校验 bundle ↔ input_schema（required 覆盖、material/asset name 唯一、无多余类型、product/virtual_ip 非空如 required）；不调 reachability（caller 已校验可达性）。

可达性规则（scope ∧ visibility ∧ 容器可达）定义在 [reachability workflow](../../../media_materials/workflow/0001-reachability/reachability.md)，由 caller 调用。原 §3 的 AssetReachability / Check*Accessible 调用迁至 caller。

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

media_workflow 采用 bundle 模式（见 [media_workflow spec](../../spec/media_workflow/impl.md) §6），不直读 materials 库；caller（BFF）在 SubmitJob 前调 [media_materials reachability workflow](../../../media_materials/workflow/0001-reachability/reachability.md) 校验可达性 + 组装 bundle。media_workflow 仅调：

- `BatchGetAssetKind(names) -> (kinds, invalid)`--Workflow 校验期校验 slot.kind / output_asset_kind 已注册、resource_type / media_type 匹配（§2）。
- `UpsertGeneratedAsset(...)`--StepRun SUCCEEDED 时落产物（归 execution，见 [workflow_execution](../0002-workflow_execution/workflow_execution.md)）。

> `AssetReachability` / `CheckProductAccessible` / `CheckVirtualIPAccessible` 由 caller 调 reachability workflow，**不归 media_workflow**（见 §3 职责在 caller）。

## 关联 domain
- [workflow](../../domain/0002-workflow/workflow.md)
- [template](../../domain/0001-template/template.md)
- [execution](../../domain/0003-execution/execution.md)（Submit 阶段复用）

## 关联术语
[Workflow Validation](../../CONTEXT.md)、[Asset Reachability](../../media_materials/CONTEXT.md)。
