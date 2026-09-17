# media_workflow 模板 / 工作流 / 执行上下文

基于 Scope 与 Template 的工作流定义、校验与执行微服务：负责管理 Template、Workflow、Step、StepBinding 的存储与版本，提供 workflow 结构校验（无环 / 单 sink / 引用合法性），以及把 Workflow 转为可调度的 Execution（Job + StepRun）。通过 `media_materials` 客户端校验 asset 引用与可达性；对内被发布计划（见 `media_materials` 后续规划）触发。归 [media_auth/CONTEXT.md](../media_auth/CONTEXT.md) 全局鉴权。对外以 Hertz + Protobuf IDL 暴露，自治数据库。

## 业务术语词典

### Workspace 隔离边界

**Workspace（工作空间）**:
Template、Workflow 和 Execution 的业务资源隔离归属；同一 Execution 的输入与产物必须属于同一 Workspace。
_禁用替代词_：Scope（Scope 是业务线分类，不是租户）

### 范围

**Scope（业务范围）**:
见 [共享资源访问语言](../resource_access/CONTEXT.md)。本服务所有 Template / Workflow 强制绑定一个 Scope；跨 Scope 引用拒绝。

### 资源权限（Template / Workflow 通用）

资源级 ACL 统一语义见 [共享资源访问语言](../resource_access/CONTEXT.md)，Grant 数据和最终授权仍归本服务。

**My Permission On Resource（资源访问权限）**:
本服务在每次读写 Template / Workflow 前推导 `MyPermission(user, resource)`，遵循全局规则。
_禁用替代词_：role、claim。

**Visibility On Resource（资源可见性档位）**:
Template / Workflow 均支持 `private | team | public`，与 grants 联动派生 [My Permission](../resource_access/CONTEXT.md#my-permission有效资源权限)。

**Template Grant / Workflow Grant（资源授权）**:
`(template_id | workflow_id, team_id) → permission ∈ {viewer, editor}` 显式授权。本表由 owner / editor 调用 service 写入。

### 模板

**Template（提示词模板）**:
调用一次 prompt / 生成动作的声明式定义。包含 inputs[]（输入资源槽位列表，每槽位引用一个 asset_kind）+ 单一 `output_asset_kind`（产出资源类别）+ prompt_body（用 slot 名作为占位符的字符串）+ `platform`/`gen_way`/`gen_type`（生成路由）；绑定 1 个 scope；含 visibility/grants/status（draft/published/archived）+ version。一次模板执行恒定产出**一个** generated_asset。归 `template` domain。
_禁用替代词_：prompt（保留给模板里的"提示词字符串"）、prompt_template、recipe。

**Template Slot（模板槽位）**:
Template inputs[] 中的一项，描述一个 `(name, kind, required)` 元组——`kind` 引用 [asset_kind](../media_materials/domain/0005-asset_kind/asset_kind.md) name，决定该槽位的 resource_type / media_type / 落表；调用方需为每个 required slot 提供对应资源 id，渲染时 slot.name 与 prompt_body 中的占位符对齐。output 不是 slot，是 Template 上的单一 `output_asset_kind` 字段。归 `template` domain。
_禁用替代词_：argument、parameter、variable。

**Template Version（模板版本号）**:
Template 的不可变快照，通过单调递增 version 区分，update 时新建 +1 的 version（保留旧可查询）。归 `template` domain。

### 工作流

**Workflow（工作流）**:
一组有序 Step + WorkflowInput + output_step_key；通过每个 Step 的 StepBinding 形成 DAG；归 1 个 scope；含 visibility/grants/status + version。归 `workflow` domain。
_禁用替代词_：flow、pipeline、dag。

**Workflow Input（工作流入口参数）**:
Workflow 顶部声明的外部输入 entry，每条形如 `(name, type, required)`，`type ∈ {material, asset, product, virtual_ip}`——运行时由调用方在 SubmitJob 时提供对应 id（material_id / generated_asset_id / product_id / virtual_ip_id），被 Step 的 `workflow_input` 引用。product / virtual_ip 输入在执行时按所绑 template slot 的 `kind` 解析为具体 material/generated_asset。归 `workflow` domain。
_禁用替代词_：argument、variable、param。

**Workflow Input Type（工作流入口参数类型）**:
Workflow Input 的 `type` 字段，枚举 `{material, asset, product, virtual_ip}`，决定调用方在 SubmitJob 时传哪种 id。与 media_materials 的 [Resource Type](../media_materials/CONTEXT.md) 区分：本类型是**入口载体类型**（传哪种 id），Resource Type 是 asset_kind 的**物理落表类别**（落哪张表）；二者仅 material/asset 两档同名且 1:1 对应，product/virtual_ip 输入经 ResolveAssetByKind 解析后，落点由所绑 slot.kind 的 Resource Type 决定。
_禁用替代词_：input_kind、carrier、source_type。

**Step（步骤）**:
Workflow 中一个节点，引用一个 Template；通过 StepBinding 声明 inputs 来源。归 `workflow` domain。

**Step Binding（步骤绑定）**:
Step 输入到其来源的映射，`source ∈ {workflow_input, step_output, const}`；每条 `(to_input, source, ref?, value?)`。归 `workflow` domain。
_禁用替代词_：edge、link、wire。

**Output Step Key（输出步骤 key）**:
Workflow 中产出最终结果的 Step 的 `step.key`；同时是 DAG 的唯一 sink；校验时强制 sink 唯一且与本值一致。归 `workflow` domain。

### 执行

**Execution（执行）**:
调用一个 Workflow 的一次实例化，落地为一个 Job；归 1 个 workflow_id + 1 个 scope（沿用 workflow.scope）+ 调用方提供的 WorkflowInput 值；同时落地一组 StepRun。归 `execution` domain。
_禁用替代词_：run、instance、invocation。

**Job（执行任务）**:
Execution 的持久化状态，状态机 `pending → running → (succeeded | failed | cancelled)`；携带 execution 的全部入参快照与最终产出。归 `execution` domain。
_禁用替代词_：task（task 留给未来通用调度）、record。

**Step Run（步骤运行）**:
Job 中某一个 Step 的执行实例，落地该 Step 的实际渲染入参、唯一最终输出、状态与失败原因；下游可以返回多个 Candidate，但确认前不构成多个 Step 输出。归 `execution` domain。
_禁用替代词_：step execution、step instance。

**Step Review Decision（步骤确认决定）**:
Step Run 等待多候选确认时由用户提交的 `accept(candidate)` 或 `regenerate(feedback)` 决定；accept 固定唯一输出并推进 DAG，regenerate 保留旧候选并开始新 attempt。
_禁用替代词_：Retry、Multi-output Step、Candidate

### 校验

**Workflow Validation（工作流校验）**:
不变形地把一个待保存的 Workflow 与所引 Templates / 资产做语义检查——step 引用前向、单 sink、root 反查覆盖全部 step、scope 匹配、资产可达性。归 `workflow_validation` workflow。

**Asset Reachability（素材可达性）**:
见 [media_materials/CONTEXT.md](../media_materials/CONTEXT.md#素材与素材类别)。本服务在 Workflow Validation 中调用之。
