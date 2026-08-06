---
name: workflow
description: Workflow（DAG 结构）存储与 step binding 校验
dependencies:
    - template
related_workflow:
    - workflow_validation
    - workflow_execution
---

## 一句话职责
Workflow 域负责存储「一组有序 Step 通过 StepBinding 串联而成的 DAG」+ WorkflowInput 入口参数 + output_step_key；版本化；提供只读校验（无环、单 sink、覆盖所有 step、引用前向、binding 类型合法）。归 1 个 scope。

### 不做什么
- 不持久化执行状态（归 [execution](../0003-execution/execution.md)）。
- 不渲染 prompt（归 [template](../0001-template/template.md)）。
- 不直接读 material / generated_asset（可达性由 `media_materials.AssetReachability` 委托）。

## 边界

### 做什么
- Create / Update / SoftDelete Workflow，强制 1:1 scope（与 Template 一致，不可改）。
- 维护 WorkflowInput[] 入口参数（`(name, type, required)`），`type ∈ {material, asset, product, virtual_ip}`——调用方在 SubmitJob 时传对应 id（material_id / generated_asset_id / product_id / virtual_ip_id）；具体 asset_kind 约束与解析选择由所绑定的 template slot 的 `kind` 承担。
- 维护有序 Step 列表（用户提交顺序即 step_order），每个 Step 引用一个 Template。
- 维护每个 Step 的 StepBinding 列表，`source ∈ {workflow_input, step_output, const}`。
- 维护 output_step_key，校验时强制它是 DAG 的唯一 sink。
- 版本化：update 时 version + 1。
- 提供只读 `Validate` 接口，返回根节点、推得的边、错误原因；不写库。

### 不做什么
- 不在 Workflow save 时执行渲染（这是 execution 的事）。
- 不持久化 const binding 的实际值是否合法（校验时由 `Template.Render` 用 ResolvedSlot 反推）。

## 能力清单

| 能力 | 说明 | 对外接口 |
|:---|:---|:---|
| CreateWorkflow | 提交 DAG 定义并 validate；创建者 = owner，visibility=private | `POST /api/v1/workflows` |
| UpdateWorkflow | 整体覆盖 steps + inputs + output_key；version+1；editor+ | `PUT /api/v1/workflows/{id}` |
| GetWorkflow | 取最新 version；viewer+ | `GET /api/v1/workflows/{id}` |
| ListWorkflows | 按 scope/name/status/visibility 过滤分页；返回 viewer+ | `GET /api/v1/workflows?scope=&name=&status=&visibility=` |
| ValidateWorkflow | 不保存，仅校验，返回 roots + edges + 错误；需 viewer+（或由 editor+ 触发） | `POST /api/v1/workflows/validate` |
| ArchiveWorkflow | 改为 archived；editor+ | `POST /api/v1/workflows/{id}/archive` |
| PublishWorkflow | draft -> published；owner | `POST /api/v1/workflows/{id}/publish` |
| GrantWorkflow | 写一条 workflow_grants；owner | `POST /api/v1/workflows/{id}/grants` |
| RevokeWorkflow | 删一条 workflow_grants；owner | `DELETE /api/v1/workflows/{id}/grants/{team_id}` |
| ChangeVisibility | 改 visibility；owner | `POST /api/v1/workflows/{id}/visibility` |

## 权限规则

按 [My Permission](../../../resource_access/CONTEXT.md#my-permission有效资源权限) 推导：

| 操作 | 所需 my_permission |
|:---|:---|
| `CreateWorkflow` | 无；创建者即 owner |
| `GetWorkflow` / `ListWorkflows` | ≥ viewer |
| `UpdateWorkflow` | ≥ editor |
| `ArchiveWorkflow` | ≥ editor |
| `DeleteWorkflow` | owner |
| `GrantWorkflow` / `RevokeWorkflow` | owner |
| `ChangeVisibility` | owner |
| `SubmitJob`（把 workflow 提交为 Job） | ≥ viewer（可读 → 可执行） |

校验时机：每个 service 方法入口先 `LoadMyPermission(userID, workflowID)` → 不足即拒绝。

### 跨资源权限一致性

`UpdateWorkflow` 时若新增 step 引用了 Template `(template_id, version)`，调用方必须同时 **viewer+** on each referenced Template；否则 workflow_validation 拒绝 `TEMPLATE_INACCESSIBLE`，workflow 不写库。模板侧可视性由模板自己的 grants 决定，与 workflow.grants 独立。

## 数据模型

### Tables — `workflows`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | bigserial | 主键 |
| workspace_id | text not null | 所属 Workspace；跨服务 ID，不建外键 |
| scope | text not null | 业务范围；1:1；不可改 |
| name | text not null | 工作流名；(scope,name,version) 唯一 |
| description | text nullable | 描述 |
| status | text not null | draft/published/archived |
| visibility | text not null | 可见性；private / team / public；默认 private |
| version | int not null | 版本号；默认 1，update 自增 |
| output_step_key | text not null | 唯一 sink 的 step.key |
| created_by | text not null | 拥有者 user_id；拥有 owner 权限 |
| created_at / updated_at / deleted_at | timestamptz | 时间戳 |

约束：UNIQUE `(scope, name, version)` WHERE `deleted_at IS NULL`；INDEX `(scope, status, updated_at)`；INDEX `(created_by)`。

### Tables — `workflow_grants`

`(workflow_id, team_id) → permission`，对应 [Resource Grant](../../../resource_access/CONTEXT.md#resource-grant资源授权)。

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| workflow_id | bigint not null | FK；复合 PK 一段；ON DELETE CASCADE |
| team_id | text not null | 团队 id；复合 PK 二段 |
| permission | text not null | viewer / editor；CHECK in ('viewer','editor') |
| granted_by | text not null | 授权者；仅 owner 可写 |
| granted_at | timestamptz | 时间戳 |

约束：PK `(workflow_id, team_id)`；INDEX `(team_id)`。

### Tables — `workflow_inputs`

Workflow 顶部声明的入口参数。每个 workflow_input 是一个**资源引用**，`type` 决定调用方在 SubmitJob 时传哪种 id；具体的 resource_type / media_type 约束由所绑定的 template slot 的 `kind` 承担（slot.kind 还充当 product / virtual_ip 输入的解析选择器，见 [workflow_execution](../../workflow/0002-workflow_execution/workflow_execution.md)）。

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | bigserial | 主键 |
| workflow_id | bigint not null | FK；ON DELETE CASCADE |
| version | int not null | 版本号 |
| name | text not null | 入口参数名；同 workflow+version 内唯一 |
| type | text not null | 引用类型；CHECK in ('material','asset','product','virtual_ip')；asset=generated_asset |
| required | bool not null | 是否必填 |
| display_order | int not null | 排序 |

约束：UNIQUE `(workflow_id, version, name)`。

### Tables — `workflow_steps`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | bigserial | 主键 |
| workflow_id | bigint not null | FK；ON DELETE CASCADE |
| version | int not null | 版本号 |
| step_key | text not null | 节点名；同 workflow+version 唯一；与 output_step_key 关联 |
| step_order | int not null | 渲染顺序；1..N |
| template_id | bigint not null | 引用的 Template；必须存在于同 scope |
| template_version | int not null | 引用的 Template 版本；不可变快照；语义同 `Job` 携带的 template_version |

约束：UNIQUE `(workflow_id, version, step_key)`；UNIQUE `(workflow_id, version, step_order)`；INDEX `(template_id, template_version)`。

### Tables — `step_bindings`

`StepBinding` 与 step 同生命周期。一个 step 可有多条 binding（多输入）。

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | bigserial | 主键 |
| step_id | bigint not null | FK；ON DELETE CASCADE |
| to_input | text not null | 消费侧槽名；引用对应 Template 的 inputs[].name |
| source | text not null | workflow_input / step_output / const；CHECK in 三选一 |
| ref_workflow_input_id | bigint nullable | 当 source=workflow_input；FK → workflow_inputs.id |
| ref_step_key | text nullable | 当 source=step_output；引用同 workflow 内更靠前的 step.step_key；该 step 产出单一 asset，无需指定 output 槽位 |
| const_value | jsonb nullable | 当 source=const；字面值；类型与 target slot.resource_type 对齐由 Render 层校验 |
| display_order | int not null | 排序 |

约束：CHECK `(source, ref_* , const_value)` 三选一互斥；UNIQUE `(step_id, to_input)`。

> 三选一约束用 CHECK 表达：
> ```sql
> CHECK (
>   (source = 'workflow_input' AND ref_workflow_input_id IS NOT NULL AND ref_step_key IS NULL AND const_value IS NULL) OR
>   (source = 'step_output'    AND ref_step_key IS NOT NULL         AND ref_workflow_input_id IS NULL AND const_value IS NULL) OR
>   (source = 'const'          AND const_value IS NOT NULL          AND ref_workflow_input_id IS NULL AND ref_step_key IS NULL)
> )
> ```

## StepBinding 值对象

```go
type BindingSource string

const (
    BindingSourceWorkflowInput BindingSource = "workflow_input"
    BindingSourceStepOutput    BindingSource = "step_output"
    BindingSourceConst         BindingSource = "const"
)

type StepBinding struct {
    ToInput            string
    Source             BindingSource
    RefWorkflowInputID int64  // 仅 source=workflow_input
    RefStepKey         string // 仅 source=step_output
    ConstValue         any    // 仅 source=const
    DisplayOrder       int
}
```

## 实体

```go
type Workflow struct {
    ID              int64
    Scope           string
    Name            string
    Description     string
    Inputs          []WorkflowInputDef
    OutputStepKey   string
    Status          WorkflowStatus
    Visibility      ResourceVisibility // 引用 media_auth 同一定义
    Version         int
    Steps           []Step
    CreatedBy       string
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

type WorkflowInputDef struct {
    Name         string
    Type         WorkflowInputType  // asset / product / virtual_ip
    Required     bool
    DisplayOrder int
}

type WorkflowInputType string
const (
    WorkflowInputTypeMaterial  WorkflowInputType = "material"   // materials 表
    WorkflowInputTypeAsset     WorkflowInputType = "asset"      // generated_assets 表
    WorkflowInputTypeProduct   WorkflowInputType = "product"
    WorkflowInputTypeVirtualIP WorkflowInputType = "virtual_ip"
)

type Step struct {
    ID              int64
    Key             string
    Order           int
    TemplateID      int64
    TemplateVersion int
    Bindings        []StepBinding
}

type WorkflowStatus string
const (
    WorkflowStatusDraft     WorkflowStatus = "draft"
    WorkflowStatusPublished WorkflowStatus = "published"
    WorkflowStatusArchived  WorkflowStatus = "archived"
)
```

## 校验细则

不变式（创建 / 更新均执行）：

1. `Scope != ""`，且 workflow 创建后不可改 scope。
2. **DAG 合法性**：
   - `step_key` 在 steps 内唯一。
   - `OutputStepKey` 必须等于 `findSink().step_key`，其中 `findSink` 由依赖关系推导（无任何 binding ref 指向它）。
   - 不允许多 sink：所有 step 中只有 `OutputStepKey` 是 sink；其它任何 step 若没人 ref 它的输出 → reject (`multiple sinks`)。
   - 不允许离群节点：从 `OutputStepKey` 反向 DFS 必须覆盖全部 step。
   - **天然无环**：因 `source=step_output` 仅允许 ref **之前的** step_key（按 step_order 严格小于当前 step.order 判定）。
3. **binding 合法性**：
   - `source=workflow_input`：`RefWorkflowInputID` 必须指向同 workflow 内一条 workflow_input。目标 slot 的 `kind`（asset_kind）的 `resource_type` 必须与 workflow_input.`type` 自洽：
     - `type=material` → slot.kind.resource_type = MATERIAL
     - `type=asset` → slot.kind.resource_type = ASSET
     - `type ∈ {product, virtual_ip}` → slot.kind.resource_type ∈ {MATERIAL, ASSET}（从 product / virtual_ip 解析一个 material 或 generated_asset）；slot.kind 充当解析选择器（见 [workflow_execution](../../workflow/0002-workflow_execution/workflow_execution.md)）
     - slot.kind 必填，否则 reject `ErrSlotKindRequired`
   - `source=step_output`：`RefStepKey` 必须存在，且 ref step 的 `StepOrder < 当前 StepOrder`；消费侧 `to_input` 槽的 kind.media_type 必须 == ref step 所引 Template 的 `OutputAssetKind.media_type`（类型安全的边）。
   - `source=const`：`ConstValue` 非 nil；类型校验在 Render 层与 slot.kind.media_type 对齐（见 [template §Render 接口](../0001-template/template.md#render-接口)）。
   - `to_input` 必须是所引 Template 的 `Inputs[].Name` 之一。
4. **Required 必填**：所有 `template.inputs[i].required=true` 在该 step 的 Bindings 里至少出现一条，且 `ConstValue` 非 nil（`source=const` 也算满足 required，等同于"调用方在 step 上常量化一个固定值"）。
5. Template 引用：`Template` 必须存在同 scope，未被 archived，且 step 引用的 `TemplateVersion` 仍存在。
6. **scope 匹配**：所有 binding 解析出的 material / asset / product / virtual_ip 必须与 workflow 同 scope——由 `workflow_validation` 跨 service 校验（material / asset 走 AssetReachability；product / virtual_ip 输入走各自的可达性规则）。
7. `Status` 流转：`draft ↔ published`（`PublishWorkflow` / 回退）`-> archived`（`ArchiveWorkflow`，终态不可逆）；不允许 archived 恢复。

### Validate（不保存）

```go
type ValidationResult struct {
    Valid           bool
    Roots           []string
    OutputStepKey   string
    DerivedEdges    []DerivedEdge
    RejectReasons   []string // 失败时填
}
type DerivedEdge struct {
    FromStep string
    ToStep   string
    ToInput  string
}
```

仅返回 roots + 推导的边，**不动数据**。

## 关联关系

### 被依赖的 domain
- [execution](../0003-execution/execution.md) — Job 携带 workflow_id + workflow_version

### 参与的 workflow
- [workflow_validation](../../workflow/0001-workflow_validation/workflow_validation.md) — 资产可达性 / scope 一致性在该 workflow 完成
- [workflow_execution](../../workflow/0002-workflow_execution/workflow_execution.md) — 实际执行时按 step 顺序、binding 解析、调用 Template.Render

### 关联术语
[Workflow Input](../../CONTEXT.md)、[Step](../../CONTEXT.md)、[Step Binding](../../CONTEXT.md)、[Output Step Key](../../CONTEXT.md)。
