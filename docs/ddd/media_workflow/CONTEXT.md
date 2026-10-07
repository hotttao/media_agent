# media_workflow 模板 / 工作流 / 执行上下文

基于 Scope 与 Template 的工作流定义、校验与执行微服务：负责管理 Template、Workflow、Step、SlotBinding 的存储与版本，提供 workflow 结构校验（无环 / 单 sink / 引用合法性），以及把 Workflow 转为可调度的 Execution（Job + StepRun）。通过 `media_materials` 客户端校验 asset 引用与可达性；对内被发布计划（见 `media_materials` 后续规划）触发。归 [media_auth/CONTEXT.md](../media_auth/CONTEXT.md) 全局鉴权。对外以 Hertz + Protobuf IDL 暴露，自治数据库。

## 业务术语词典

### Workspace 隔离边界

**Workspace（工作空间）**:
Execution 的业务执行隔离归属。输入和产物必须匹配该 Workspace 所属企业；企业内共享素材可复用于同企业的不同 Workspace，不要求素材归某个 Workspace。Template、Workflow 的 Workspace 资源归属仍是目标模型，不代表当前定义表已实现该隔离。
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
调用一次 prompt / 生成动作的声明式定义。包含 inputs[]（输入资源槽位列表，每槽位引用一个 asset_kind）+ 单一 `output_asset_kind`（产出资源类别）+ prompt_body（用 slot 名作为占位符的字符串）+ 不可变 Execution Capability 引用；绑定 1 个 scope；含 visibility/grants/status（draft/published/archived）+ version。一次模板执行恒定产出**一个** generated_asset。归 `template` domain。
_禁用替代词_：prompt（保留给模板里的"提示词字符串"）、prompt_template、recipe。

**Template Slot（模板槽位）**:
Template inputs[] 中的一项，描述一个 `(name, kind, required)` 元组——`kind` 引用 [asset_kind](../media_materials/domain/0005-asset_kind/asset_kind.md) name，决定该槽位的 resource_type / media_type / 落表；调用方需为每个 required slot 提供对应资源 id，渲染时 slot.name 与 prompt_body 中的占位符对齐。output 不是 slot，是 Template 上的单一 `output_asset_kind` 字段。归 `template` domain。
_禁用替代词_：argument、parameter、variable。

**Template Version（模板版本号）**:
Template 的不可变快照，通过单调递增 version 区分，update 时新建 +1 的 version（保留旧可查询）。归 `template` domain。

**Execution Retry（执行失败恢复）**:
继续一个失败Execution中尚未成功的任务；从某个节点发起也作用于所属Execution，不表示仅重跑该节点及其下游。它不同于审批重新生成或改变输入重新开始。
_禁用替代词_：Regeneration、Execution Start、整单重新生成。

**Execution Capability（执行能力）**:
可被生成 Worker 实现的、带版本的Provider操作契约，例如Doubao生图、Doubao生视频、即梦生图；声明固定输入与技术输出媒体类型，不表示模特图、定妆图、首帧等业务用途。Template引用已发布能力并决定业务槽位、提示词映射与最终Asset Kind。
_禁用替代词_：Conductor Task、Worker、Template

**Generation Method（生成方式）**:
Capability输入到输出的媒体转换方式，例如文生图(t2i)、图生图(i2i)、文生视频(t2v)、图生视频(i2v)；字段统一称为`generation_method`。它不表示API、网页或CLI调用手段，也不表示模特图等业务用途。
_禁用替代词_：gen_way、Provider调用方式、业务节点名称

**Generation Type（生成类型）**:
同一生成方式下固定的输入模式，例如首帧加提示词(first_frame_prompt)、多图加提示词(multi_image_prompt)。它不表示模特图、定妆图等业务用途，也不等同于输出媒体类型。
_禁用替代词_：Asset Kind、generation_method、业务步骤

**Capability Worker（能力执行器）**:
实现 Execution Capability 的执行组件。`personal_worker` 运行在用户环境并经 Worker Gateway API 领取 User Execution Domain；`managed_worker` 运行在 media_workflow 内部并领取 Managed Execution Domain。二者都不承担模板渲染、Workflow 编译或业务资源授权。
_禁用替代词_：Template Renderer、Conductor Worker、Worker Instance

**Execution Target（执行目标）**:
Execution Capability Version 冻结的执行者类别，取值 `personal_worker | managed_worker`。它决定 Job 启动时的 Task Domain 类型，不指定具体 Worker 实例；已发布版本不得修改，也不得在某类 Worker 离线时隐式回退。
_禁用替代词_：Worker ID、Deployment、Fallback Queue

**Personal Worker（个人执行器）**:
User 拥有的委托 Worker Actor，由 `media_worker` 客户端承载；使用绑定 `Worker:<id>` 的 API Token，经 Gateway 获得 `sub=User`、`act.sub=Worker` 的 Internal Identity JWT，只能领取该 User 的 Personal Execution Domain。
_禁用替代词_：Managed Worker、Agent、Worker Instance

**Managed Worker（托管执行器）**:
由 `media_workflow` 服务边界内运行的 Capability Worker，可作为独立进程部署但不经过公开 Worker API。它不申请 API Token、不请求 Talos，只轮询配置允许的 Managed Execution Domain 和 Capability；可信 `worker_id` 来自服务配置，`worker_instance_id` 只用于运行审计。
_禁用替代词_：Personal Worker、Delegated Worker、External Worker

**Media Worker（媒体执行客户端）**:
承载一个或多个 Personal Worker Capability 的客户端工具或常驻服务；携带 API Token 经统一网关调用稳定的 Worker Gateway API，以 `worker_instance_id` 标识运行实例，并声明支持的 Personal Execution Capability Version。联调时可以启用显式 Mock Profile，但生产 Profile 必须调用真实本地能力。
_禁用替代词_：media_workflow、Provider 服务、Mock Server

**Worker Gateway API（客户端执行网关接口）**:
由 `media_workflow` 提供、经统一网关认证的客户端任务协议，负责 Capability 注册、任务领取、心跳、完成与失败。它把 Execution Capability 映射为内部 Conductor Task Definition，并隐藏 Conductor task/workflow 标识。
_禁用替代词_：Conductor Task API、Provider API、Browser API

**Execution Domain（执行路由域）**:
`media_workflow` 在启动 Conductor Workflow 时依据 Capability Version 的 Execution Target 设置的领取隔离键。Personal Domain 从 Job User 派生；Managed Domain 从受控 Worker Pool 配置派生。该值只由服务端计算，客户端不能指定，两类 Domain 不能相互回退，也不能回退到无域队列。
_禁用替代词_：Organization、Workspace、Scope、Conductor Queue

**Invocation Mapping（调用映射）**:
Template Version 中把业务 Template Slot 转换为 Execution Capability 固定输入参数的声明式规则；例如把渲染结果映射到 `prompt`，把 `person`、`garment` 的资源引用按确定顺序组成 `images[]`。映射必须在 Workflow 发布时完成类型检查，禁止 Worker 根据 Slot 名猜测参数。
_禁用替代词_：Worker 参数拼装、动态入参、prepare plan

### 工作流

**Workflow（工作流）**:
一组有序 Step + WorkflowInput + output_step_key；通过每个 Step 的 SlotBinding 形成 DAG；归 1 个 scope；含 visibility/grants/status + version。归 `workflow` domain。
_禁用替代词_：flow、pipeline、dag。

**Workflow Input（工作流入口参数）**:
Workflow 顶部声明的外部输入 entry，每条形如 `(name, type, asset_kind?, required)`，`type ∈ {material, asset, product, virtual_ip}`——运行时由调用方在 SubmitJob 时提供对应 id（material_id / generated_asset_id / product_id / virtual_ip_id），被 Step 的 `workflow_input` 引用。material / asset 输入必须冻结唯一 `asset_kind`，供调用方选择兼容资源并由服务端校验；product / virtual_ip 输入在执行时按所绑 Template Slot 的 `kind` 解析为具体 material/generated_asset。归 `workflow` domain。
_禁用替代词_：argument、variable、param。

**Workflow Input Type（工作流入口参数类型）**:
Workflow Input 的 `type` 字段，枚举 `{material, asset, product, virtual_ip}`，决定调用方在 SubmitJob 时传哪种 id。与 media_materials 的 [Resource Type](../media_materials/CONTEXT.md) 区分：本类型是**入口载体类型**（传哪种 id），Resource Type 是 asset_kind 的**物理落表类别**（落哪张表）；二者仅 material/asset 两档同名且 1:1 对应，product/virtual_ip 输入经 ResolveAssetByKind 解析后，落点由所绑 slot.kind 的 Resource Type 决定。
_禁用替代词_：input_kind、carrier、source_type。

**Step（步骤）**:
Workflow 中一个节点，引用一个 Template；通过 SlotBinding 声明 Template Slot 的来源。归 `workflow` domain。

**Slot Binding（槽位绑定）**:
Step 的 Template Slot 到其来源的映射，`source ∈ {workflow_input, step_output, const}`；每条 `(to_slot, source, ref?, value?)`。归 `workflow` domain。
_禁用替代词_：edge、link、wire。

**Step Approval Policy（步骤审批策略）**:
Step 对生成 Candidate 的人工审批要求，取值 `when_multiple_candidates | always`。多个 Candidate 无论配置如何都必须审批；`always` 还要求单个 Candidate 进入审批。0 个 Candidate 直接失败，不进入审批。
_禁用替代词_：Multi-output、Optional Review、Skip Multi-candidate Review

**Output Step Key（输出步骤 key）**:
Workflow 中产出最终结果的 Step 的 `step.key`；同时是 DAG 的唯一 sink；校验时强制 sink 唯一且与本值一致。归 `workflow` domain。

**Workflow Compilation（工作流编译）**:
把已发布 Workflow 的不可变版本转换为 Conductor Workflow Definition，并保存源版本、目标 name/version 与 definition_hash 的映射；编译结果是执行适配物，不是新的业务 Workflow。
_禁用替代词_：Workflow Execution、DAG Save

**Workflow Publication Intent（工作流发布意图）**:
拥有者请求发布某个不可变Workflow版本的持久事实，关联冻结编译与注册契约；发布尚在恢复中不等于该版本已经可执行。
_禁用替代词_：Job、Workflow Execution、Compilation本身。

**Semantic Compiler（语义编译器）**:
把 Workflow、Template Version、Invocation Mapping 与 Execution Capability Version 解析并校验为 Executable Workflow IR；负责业务引用解析、类型检查和版本冻结，不生成 Conductor 专有结构。
_禁用替代词_：DAG Compiler、Conductor Compiler

**Executable Workflow IR（可执行工作流中间表示）**:
Semantic Compiler 的不可变输出，也是 Conductor Backend Compiler 的唯一输入；由 `ExecutableWorkflowIR` 与其中的 `ExecutableStepIR` 组成，描述依赖、模板渲染上下文、Capability 调用映射、唯一输出和执行策略。它是编译产物，不是编辑源或新的业务聚合。
_禁用替代词_：CompiledStepPlan、Workflow DTO、Conductor Definition

**Conductor Backend Compiler（Conductor 后端编译器）**:
把 Executable Workflow IR 降低为 Conductor Workflow Definition。它自动插入 `media_render_template_v1`、Capability Task、候选确认与结果收口等技术任务，但不重新解释业务 Workflow。
_禁用替代词_：Semantic Compiler

**Template Rendering Task（模板渲染任务）**:
编译器自动插入的内部通用任务 `media_render_template_v1`；固定契约为 `template:string + context:map<string,json> -> prompt:string`。它不是 Execution Capability，不选择 Provider，也不猜测图片数组顺序。
_禁用替代词_：prepare_step、Execution Capability

**Internal Execution Task（内部执行任务）**:
由 `media_workflow` 自己拥有并执行的 Conductor 技术任务，例如模板渲染、已接受产物登记与唯一输出收口。它不属于 Execution Capability，也不得被下发给客户端 Media Worker。
_禁用替代词_：Capability Worker

**Topological Wave（拓扑波次）**:
由 Workflow DAG 按依赖关系计算的一组可并行 Step。编译器在相邻波次之间设置屏障；单 Step 波次编译为一个 Step 子工作流，多 Step 波次编译为动态并行分支后 JOIN。
_禁用替代词_：Batch、Stage（除非专指展示层）

### 执行

**Execution Retry Epoch（执行重试周期）**:
历史整次重跑记录所使用的周期标记，不用于决定当前执行能否继续或哪些节点应运行；原生失败继续不创建新的业务周期。它不同于候选生成轮次或Worker投递次数。
_禁用替代词_：Generation、Dispatch Attempt、Review Delivery Attempt。

**Generation Result（生成结果）**:
一个 StepRun 在某轮能力生成后由用户选定或自动接受的不可变结果；它与该轮 Generated Asset 对应，不等同于稳定节点身份或 Worker 消息投递次数。
_禁用替代词_：StepRun、Worker Result Receipt、最新产物别名。

**Execution（执行）**:
调用一个 Workflow 的一次实例化，落地为一个 Job；归 1 个 workflow_id + 1 个 scope（沿用 workflow.scope）+ 调用方提供的 WorkflowInput 值；同时落地一组 StepRun。归 `execution` domain。
_禁用替代词_：run、instance、invocation。

**Imported Incomplete Execution（导入的未完成执行）**:
从旧系统有完整上游关联、但尚未产生最终视频的图片谱系恢复出的 Execution。已存在的 ModelImage、StyleImage、FirstFrame 作为带来源标记的已完成 Step Run，后继步骤等待显式继续执行；它不是一次新提交的完整运行，也不等同于断链记录。归 `execution` domain。
_禁用替代词_：孤儿素材、已完成 Job、自动重跑。

**Imported Completed Execution（导入的已完成执行）**:
从旧系统一条已产出 Video 且上游关系完整的生成链恢复出的终态 Execution；一条 Video 对应一条 Execution，共用的上游图片仍是同一批来源资产。VideoPush 是多条 Video 的下游业务事实，不是这条 Execution 的第二个输出。归 `execution` domain。
_禁用替代词_：VideoPush Workflow、重新生成的执行、伪造 Conductor 实例。

**Historical VideoPush（历史发布记录）**:
从旧系统保留的多视频组合、发布文字、媒体与发布状态事实；按来源 VideoPush 身份独立存在，并引用各成员 Video 的已导入 Execution。它不是当前生成 Workflow 的 Step，也不表示新系统已执行剪辑或发布。
_禁用替代词_：Workflow 输出、Video Step、重新发布任务。

**Resume Execution（继续执行）**:
用户对 Imported Incomplete Execution 发起的显式命令：补齐所缺 Workflow Input 后，沿已保存的图片结果从首个未完成 Step 继续，不重新调用已完成阶段的 Capability。归 `workflow_execution` application workflow。
_禁用替代词_：Retry、重新 SubmitJob、恢复旧 Conductor 实例。

**Frozen Storage Reference（冻结对象引用）**:
Execution 输入快照与启动命令中保存的企业命名空间内 Object Path；短期签名 URL 只用于输入校验和任务交付，不属于冻结的执行事实。
_禁用替代词_：持久化签名 URL、Provider URL。

**Text-only Movement Input（纯文本动作输入）**:
从用户有权访问的 Text-only Movement 冻结非空动作内容、且不带媒体引用的 Workflow Material Input；模板可渲染其内容，Capability 图片或视频引用不得从空 URL 推断生成。
_禁用替代词_：缺失输入、虚构动作视频、空 Object Path。

**Job（执行任务）**:
Execution 的持久化状态，状态机 `pending → running → (succeeded | failed | cancelled)`；携带 execution 的全部入参快照与最终产出。归 `execution` domain。
_禁用替代词_：task（task 留给未来通用调度）、record。

**Step Run（步骤运行）**:
Job 中某一个 Step 的执行实例，落地该 Step 的实际渲染入参、唯一最终输出、状态与失败原因；下游可以返回多个 Candidate，但确认前不构成多个 Step 输出。归 `execution` domain。
_禁用替代词_：step execution、step instance。

**Job Submission Receipt（任务提交回执）**:
用户在指定企业及 Workspace 中一次明确提交意图所对应的已接受 Job 身份与请求语义凭据；与生成结果、审批决定和失败重试不同。
_禁用替代词_：Worker Result Receipt、Conductor Ack、Retry Record。

**Step Review Decision（步骤确认决定）**:
Step Run 按 Step Approval Policy 等待确认时由用户提交的 `accept(candidate)` 或 `regenerate(feedback)` 决定；accept 固定唯一输出并推进 DAG，regenerate 保留旧轮候选审计并开始独立的新一轮生成，不取消其他步骤。
_禁用替代词_：Retry、Multi-output Step、Candidate

**Step Execution Fragment（步骤执行片段）**:
Conductor 中承载一个业务 Step 的固定版本子工作流。它编排复用判定、执行能力调用、候选持久化、人工确认和最终单输出，不改变 Step 的领域边界。
_禁用替代词_：Business Step、Template

**Worker Dispatch（客户端任务派发）**:
一个 Conductor Capability Task 被 `media_workflow` 领取并租约式交付给某个 Media Worker 的持久记录；以不透明 `dispatch_id` 对外，绑定 Job、Step Run、attempt、User、Organization、Credential 与 Worker Instance。它与 Job/Step Run 分属不同一致性边界。
_禁用替代词_：Conductor Task、Step Run、Worker Job

**Worker Result Receipt（Worker 结果收据）**:
`media_workflow` 接收 Worker 完成或失败结果后生成的幂等收据；先持久化结果 Inbox，再异步更新 Conductor，避免 Conductor 暂时不可用导致客户端重复执行 Provider 副作用。
_禁用替代词_：Conductor Ack、Generated Asset、Step Output

### 校验

**Workflow Validation（工作流校验）**:
不变形地把一个待保存的 Workflow 与所引 Templates / 资产做语义检查——step 引用前向、单 sink、root 反查覆盖全部 step、scope 匹配、资产可达性。归 `workflow_validation` workflow。

**Asset Reachability（素材可达性）**:
见 [media_materials/CONTEXT.md](../media_materials/CONTEXT.md#素材与素材类别)。本服务在 Workflow Validation 中调用之。
