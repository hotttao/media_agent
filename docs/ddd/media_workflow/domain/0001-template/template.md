---
name: template
description: Template（提示词模板）的存储、版本与渲染入口
dependencies:
    - media_materials.asset_kind
    - execution_capability
related_workflow:
    - workflow_validation
    - workflow_execution
---

## 一句话职责
Template 域负责存储「声明型」提示词模板：每个 Template 用 inputs[] 显式描述所需的输入资源槽位（每槽位引用一个 [asset_kind](../../../media_materials/domain/0005-asset_kind/asset_kind.md)），用单一 `output_asset_kind` 声明产出的资源类别，并把 prompt_body 作为可渲染正文；引用一个不可变 [Execution Capability](../0004-execution-capability/execution_capability.md) 版本，并以 Invocation Mapping 声明业务 Slot 如何转换为 Capability 固定参数；维护 status/version 生命周期；提供渲染入口（template + context → prompt）。
_不属于_：调度 workflow（归 [execution](../0003-execution/execution.md) 域）、管理 asset 实体（归 `media_materials`）、校验 workflow 结构（归 [workflow_validation](../../workflow/0001-workflow_validation/workflow_validation.md)）。

## 边界

### 做什么
- 创建 / 更新 / 软删 Template，强制 1:1 scope（scope 不可空，新建后不可改）。
- 维护 inputs[] 的 schema：每个元素一个 input slot，含 `name` + `kind`（引用 asset_kind name）+ `required`；name 在同一 Template 内唯一。
- 维护单一 `output_asset_kind`：一次模板执行产出**一个** asset，其类别由此字段声明（引用 asset_kind name）；media_type 由 `asset_kind.media_type` 推导，不另存。
- 维护 prompt_body：声明式正文，使用 `{{.name}}` 形式的占位符引用 inputs 的 name。
- 维护 `capability_key` / `capability_version`：引用已发布执行能力；`platform` / `gen_way` / `gen_type` 仅在旧数据迁移期保留只读映射。
- 维护 `invocation_mapping`：不可变、声明式地生成 Capability 参数。初始表达式只允许 `render_template`、`asset_ref(slot)` 与 `list(...)`，禁止任意脚本和运行时反射。
- 版本化：update 时 `version + 1`，旧版本只读保留可查。
- 提供 `Render(slot_values []ResolvedSlot)` 接口，输入每个 input slot 的解析结果，产出渲染后的 prompt_body + output_asset_kind（及其 media_type）。
- 校验 input slot.kind 与 output_asset_kind 在 asset_kind 注册表中已注册且未停用（调 `media_materials.BatchGetAssetKind`）。

### 不做什么
- 不加载 / 不存储 Asset（仅引用 asset_kind name）；asset 可达性由 workflow_validation 委托 [media_materials](../../../media_materials/CONTEXT.md) 的 `AssetReachability`。
- 不持久化 Template 的执行记录（归 [execution](../0003-execution/execution.md)）。
- 不管理 Workflow 的 DAG；只供 Workflow 的 [step](../0002-workflow/workflow.md) 引用其 ID。

## 能力清单

| 能力 | 说明 | 对外接口 |
|:---|:---|:---|
| CreateTemplate | 新建模板，初版 version=1, status=draft, visibility=private；创建者 = owner | `POST /api/v1/templates` |
| UpdateTemplate | 修改 inputs/output_asset_kind/prompt_body/capability，自增 version；需 editor+ | `PUT /api/v1/templates/{id}` |
| GetTemplate | 取最新 version 的 Template；viewer+ | `GET /api/v1/templates/{id}` |
| GetTemplateVersion | 取指定 (id, version) 快照；viewer+ | `GET /api/v1/templates/{id}/versions/{version}` |
| ListTemplates | 按 scope/asset_kind/name/visibility 过滤分页；返回 viewer+ 的集合 | `GET /api/v1/templates?scope=&asset_kind=&name=&visibility=` |
| ArchiveTemplate | 改为 archived；editor+ | `POST /api/v1/templates/{id}/archive` |
| PublishTemplate | draft -> published；owner | `POST /api/v1/templates/{id}/publish` |
| Render | 接收 slot_values，输出渲染后 prompt 与 output_asset_kind；viewer+ | `POST /api/v1/templates/{id}/render` |
| ParseSlotSpec | 仅解析 prompt_body 占位符并产出一个 draft inputs[]；无需权限 | `POST /api/v1/templates/parse` |
| GrantTemplate | 写一条 template_grants；owner | `POST /api/v1/templates/{id}/grants` |
| RevokeTemplate | 删一条 template_grants；owner | `DELETE /api/v1/templates/{id}/grants/{team_id}` |
| ChangeVisibility | 改 visibility；owner | `POST /api/v1/templates/{id}/visibility` |

## 数据模型

### Tables — `templates`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | bigserial | 主键 |
| scope | text not null | 业务范围；1:1；与 `config.SupportedScopes` 一致；不可改 |
| name | text not null | 模板名；(scope, name, version) 唯一 |
| description | text nullable | 模板说明 |
| prompt_body | text not null | 渲染正文；占位符 `{{.name}}` 引用 inputs.name |
| output_asset_kind | text not null | 产出资源类别；引用 `asset_kinds.name`，且 resource_type=ASSET；必填 |
| capability_key | text not null | 引用已发布 Execution Capability |
| capability_version | int not null | 不可变能力版本 |
| invocation_mapping | jsonb not null | Slot 到 Capability 参数的声明式映射；随 Template Version 冻结 |
| platform / gen_way / gen_type | text nullable | 旧数据迁移字段；新写接口忽略，迁移完成后删除 |
| status | text not null | 草稿/已发布/归档；draft / published / archived |
| visibility | text not null | 可见性档位；private / team / public；默认 private |
| version | int not null | 不可变版本号；默认 1；update 自增；同 (scope, name) 内单调递增 |
| created_by | text not null | 拥有者 user_id；拥有 owner 权限 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 更新时间 |
| deleted_at | timestamptz nullable | 软删时间戳 |

约束：UNIQUE `(scope, name, version)` WHERE `deleted_at IS NULL`；INDEX `(scope, status)`；INDEX `(created_by)`；INDEX `(output_asset_kind)`。

### Tables — `template_grants`

`(template_id, team_id) → permission` 显式授权，对应 [Resource Grant](../../../resource_access/CONTEXT.md#resource-grant资源授权)。

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| template_id | bigint not null | FK；复合 PK 一段；ON DELETE CASCADE |
| team_id | text not null | 团队 id（media_auth.teams.id）；复合 PK 二段 |
| permission | text not null | viewer / editor；CHECK in ('viewer','editor') |
| granted_by | text not null | 授权者；仅 owner 或 editor 可写 |
| granted_at | timestamptz | 时间戳 |

约束：PK `(template_id, team_id)`；INDEX `(team_id)`。

### Tables — `template_inputs`

Template 的输入槽位。每行一个 input slot；output 已提升为 `templates.output_asset_kind` 单字段，不在此表。

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | bigserial | 主键 |
| template_id | bigint not null, FK | 所属 template；ON DELETE CASCADE |
| version | int not null | 模板版本号；与 templates.version 联动 |
| name | text not null | slot 名；同 template+version 内唯一；在 prompt_body 中作为 `{{.name}}` 占位符 |
| kind | text not null | asset_kind 名；引用 `asset_kinds.name`；决定该槽位的 resource_type / media_type / 落表 |
| required | bool not null | 是否必填；default false |
| display_order | int not null | 渲染顺序 |

约束：UNIQUE `(template_id, version, name)`；INDEX `(template_id, version)`。

## Input Slot 值对象

```go
type Slot struct {
    Name         string   // 在 prompt_body 中作为 {{.Name}} 占位符
    Kind         string   // asset_kind name；决定 resource_type / media_type / 落表
    Required     bool     // 默认 false
    DisplayOrder int      // 仅排版有用；不影响逻辑
}
```

`inputs []Slot` 元素数 ≥ 0（允许空，但工程上建议至少声明 1 个 slot）；同一 Template 内 `Slot.Name` 不重复。

## 实体

```go
type Template struct {
    ID               int64
    Scope            string
    Name             string
    Description      string
    PromptBody       string
    OutputAssetKind  string  // 产出的 asset_kind name（resource_type=ASSET）
    CapabilityKey     string
    CapabilityVersion int
    InvocationMapping map[string]InvocationExpression
    Status           TemplateStatus
    Visibility       ResourceVisibility
    Version          int
    Inputs           []Slot  // input 槽位
    CreatedBy        string
    CreatedAt        time.Time
    UpdatedAt        time.Time
}

type TemplateStatus string
const (
    TemplateStatusDraft     TemplateStatus = "draft"
    TemplateStatusPublished TemplateStatus = "published"
    TemplateStatusArchived  TemplateStatus = "archived"
)

type ResourceVisibility string
const (
    VisibilityPrivate ResourceVisibility = "private"
    VisibilityTeam    ResourceVisibility = "team"
    VisibilityPublic  ResourceVisibility = "public"
)
```

`Visibility` 与 [共享资源访问语言](../../../resource_access/CONTEXT.md) 一致。

## 权限规则

按 [My Permission](../../../resource_access/CONTEXT.md#my-permission有效资源权限) 推导，操作权限如下：

| 操作 | 所需 my_permission |
|:---|:---|
| `CreateTemplate` | 无；创建者即 owner |
| `GetTemplate` / `ListTemplates` / `GetTemplateVersion` | ≥ viewer（含 owner / editor / viewer / public） |
| `UpdateTemplate`（修改 inputs/output_asset_kind/prompt_body/capability） | ≥ editor |
| `ArchiveTemplate` | ≥ editor |
| `DeleteTemplate`（软删） | owner |
| `GrantTemplate` / `RevokeTemplate`（写 grants 表） | owner |
| `ChangeVisibility` | owner |
| `RenderTemplate`（按 id 调用 render） | ≥ viewer |

校验时机：每个 service 方法入口先 `LoadMyPermission(userID, resourceID)` → 不足即拒绝（errno 403 / 401）。

## 校验细则

不变式（创建 / 更新均执行）：

1. `Scope != ""`；同一 `(scope, name)` 仅允许下一个待创建或下一个自增 version。
2. `Inputs` 内部 `Name` 唯一。
3. `PromptBody` 内出现的所有 `{{.name}}` 必须能在 `Inputs` 的某个 slot 找到对应 name；多余的 `{{.name}}` → reject。
4. 每个 input slot 的 `Kind` 必填，且必须指向 `asset_kinds` 注册表中已注册、status∈{SYSTEM, ENABLED} 的 kind（调 `media_materials.BatchGetAssetKind` 校验）。
5. `OutputAssetKind` 必填，且所引 asset_kind 的 `resource_type=ASSET`（产物落 generated_assets 表）；其 `media_type` 即产出的媒体形态。
6. `(CapabilityKey, CapabilityVersion)` 必须指向 published Capability，且其 output_asset_kind 与 Template 一致；新模板禁止以 legacy route 字段分发。
7. `InvocationMapping` 必须覆盖 Capability input schema 的全部 required 字段；表达式引用的 Slot 必须存在，推导类型必须兼容目标字段。`render_template` 只能生成 string，`asset_ref` 只能接受资源 Slot，`list` 生成有序数组。
8. `Status` 流转：`draft ↔ published`（`PublishTemplate` / 回退）`-> archived`（`ArchiveTemplate`，终态不可逆）；不允许 archived 恢复到 draft 或 published。
9. update 时不允许直接改 `Scope`；改 `Scope` 必须新建 Template（避免版本历史跨 scope）。
10. Archive 边界：archive 一个被运行中 Job 引用的 template 不会中断 Job（Job 持 `template_version` 不可变快照）；但新 workflow 不再能引用 archived template（workflow_validation 闸2 拒绝 `TEMPLATE_ARCHIVED`）。对已活跃 binding 的 offline scan 见 [workflow_validation](../../workflow/0001-workflow_validation/workflow_validation.md) §2（本期未实现）。

### Render 接口

```go
type ResolvedSlot struct {
    Name        string
    Source      SlotSource         // 见下
    AssetID     string             // 仅 source=asset 时有值
    AssetURL    string             // 仅 source=asset 时有值
    AssetMeta   map[string]any     // 仅 source=asset 时有值；asset.metadata 透传
    ConstValue  any                // 仅 source=const 时有值（可序列化 JSON）
}

type SlotSource string
const (
    SlotSourceAsset SlotSource = "asset" // 由 workflow_execution 调 AssetRead 填充
    SlotSourceConst SlotSource = "const" // 由 step binding const_value 直接填入
)

type RenderedTemplate struct {
    Prompt          string
    OutputAssetKind string  // = Template.OutputAssetKind
    OutputMediaType string  // = asset_kind(OutputAssetKind).media_type，供 execution 分发生成器
}
```

`Render` 必须：
1. 对每个 `input` slot：
   - 若 `ResolvedSlot` 缺失但 `Required=true` → 返回 `ErrRequiredSlotMissing{name}`。
   - 若 `source=asset`，从 `AssetURL` 或 `AssetMeta["url"]` 取可渲染字符串（实现选其一）；`AssetMeta` 其余 key 可被 prompt_body 通过 `{{.name.key}}` 引用。
   - 若 `source=const`，直接序列化 `ConstValue`（按 slot.kind 的 media_type 适配：TEXT→string、其它 marshal）。
2. 把 `PromptBody` 中每个 `{{.name}}` 或 `{{.name.key}}` 替换为该 slot 的可渲染字符串。
3. `RenderedTemplate.OutputAssetKind` 直接回填 `Template.OutputAssetKind`；`OutputMediaType` 由该 kind 的 `media_type` 给出（调 `BatchGetAssetKind` 缓存查）。
4. 资源类型适配（类型简单校验，避免 Render 在生产期炸）：`ConstValue` 为 nil 时拒收 `source=const`；其它类型不严格要求，但 prompt 渲染端若读取不到 key 应回退到空串（不要 panic）。

> `Source = asset / const` 两路分别覆盖 "binding.source = workflow_input | step_output" 与 "binding.source = const"——前者由 `media_workflow.workflow_execution` 在解析 binding 后通过 `AssetRead` 填充为 `asset`；后者直接转发。`step_output` 在前驱 step 成功后同样被填成 `asset`（拿前驱 StepRun 的单一 `output_asset_id`，调一次 AssetRead）。

## 与 media_materials 的契约

Template 域**不直接访问** Asset/material；仅引用 asset_kind name。校验 kind 合法性通过 `media_materials.BatchGetAssetKind(names)`（返回每个 kind 的 resource_type / media_type / status）。asset_kind 全局注册，无 scope，故无需按 scope 过滤。

## 关联关系

### 被依赖的 domain
- [workflow](../0002-workflow/workflow.md) — Step 引用 Template ID
- [execution](../0003-execution/execution.md) — 渲染环节读取 Slot

### 参与的 workflow
- [workflow_validation](../../workflow/0001-workflow_validation/workflow_validation.md) — 校验 template 是否存在、status 是否 archived、prompt_body 占位符是否合法
- [workflow_execution](../../workflow/0002-workflow_execution/workflow_execution.md) — 渲染环节

### 关联术语
[Scope](../../CONTEXT.md)、[Template Slot](../../CONTEXT.md)、[Template Version](../../CONTEXT.md)、[Asset Kind](../../../media_materials/CONTEXT.md)、[Resource Type](../../../media_materials/CONTEXT.md)。
