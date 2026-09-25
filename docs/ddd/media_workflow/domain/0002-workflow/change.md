# workflow domain 变更记录

## 2026-07-03 · 初始建模（media_agent v1）
- 来自 `media_ai` 的 `workflow` 设计迁入并扩展：
  - 旧版 `step.inputBindings` 的三选一语义沿用。
  - 旧版 `team_id` 字段**移除**：归属 / 创建者改为 `created_by` + 强制 `scope`；team 概念在本服务**不建模**，权限由 `media_auth` 决定。
  - `workflow_inputs` / `output_step_key` / `status` / `version` 沿用；`step_order` 由提交顺序推导为持久列。
  - `SlotBinding` 三选一互斥用 CHECK 表达。

## 修改记录

### 2026-07-21 · 补 Publish 能力 + 依赖修正
- 新增 `PublishWorkflow`（`POST /api/v1/workflows/{id}/publish`，draft -> published，owner），补全状态机 published 产生路径（原状态机有 published 态但无操作可达）。
- 校验细则新增第 7 条 status 流转：`draft ↔ published`（`PublishWorkflow` / 回退）`-> archived`（`ArchiveWorkflow`，终态不可逆）。
- `dependencies` 去掉 `scope`（已降为 shared）。

### 2026-07-03 · 第二轮反馈：`source=const` 满足 required
- **放宽** workflow.md §校验细则 第 4 条：原本约束 `binding.source 不得为 const`，现允许 const 满足 required input。
- 同步修改 [template.md §Render 接口](../0001-template/template.md#render-接口)：ResolvedSlot 加 `SlotSource = asset|const`，Render 按 source 分支处理。
- 用户理由：业务上"调用方在 step 上常量化一个固定值"（如语言、风格描述）应当天然支持，不需要每次提交都传。

### 2026-07-03 · 第三轮反馈：workflow 作为资源级 ACL 实体
- 加 `visibility ∈ {private, team, public}` 与 `workflow_grants (workflow_id, team_id, permission)`，与 template / virtual_ip / skill 一致。
- 加 GrantWorkflow / RevokeWorkflow / ChangeVisibility；SubmitJob 要求 viewer+ on workflow。
- **跨资源权限一致性**：UpdateWorkflow 时若 step 引用 (template_id, version)，调用方必须 viewer+ on each template；否则 workflow_validation 拒 `TEMPLATE_INACCESSIBLE`。
- permission 推导统一走 [media_auth §My Permission](../../../media_auth/CONTEXT.md#my-permission资源访问权限)。

### 2026-07-03 · Review 反馈：product / virtual_ip 可作 workflow 输入
- 纠正前期"workflow 输入统一是 asset slot"的表述：workflow_input 的 `type ∈ {asset, product, virtual_ip}`——product / virtual_ip 是合法的 workflow 顶层输入。
- workflow_inputs 表去掉 `resource_type` / `asset_kind` 列，改为 `type` 列；WorkflowInputDef 同步改为 `{Name, Type, Required, DisplayOrder}`。asset_kind 约束与解析选择器统一由 template slot 承担。
- step_binding source 仍为 `workflow_input / step_output / const`——product / virtual_ip **不能**作为 step_binding 的直接 source，必须先声明为 workflow_input 再被引用。
- 校验细则第 3 条加：product / virtual_ip 输入绑定的 slot 必须声明 asset_kind（解析选择器），否则 reject `SLOT_ASSET_KIND_REQUIRED`。
- 解析在 execution 阶段：`ResolveAssetByKind(owner_kind, owner_id, asset_kind)` 取唯一 asset；0 或 >1 → 报错。

### 2026-07-03 · Review 反馈：全面对齐 media_ai asset_kind 模型
- workflow_input.`type` 扩展为 `{material, asset, product, virtual_ip}`（asset=generated_asset），对齐 asset_kind.resource_type。
- step_binding 校验改为基于 slot.`kind`（asset_kind name）：slot.kind.resource_type 与 workflow_input.type 自洽；step_output 边的类型匹配改用 kind.media_type。
- `SLOT_ASSET_KIND_REQUIRED` 错误码改为 `SLOT_KIND_REQUIRED`（slot.kind 现必填，不限于 product/virtual_ip 输入）。

## 来自业务规则的强约束
1. `scope` 不可改。理由同 template。
2. `output_step_key` 必须等于推导出的唯一 sink，且为唯一 sink。
3. required input 的 binding 三种 source 都可接受；const 视同"调用方在 step 层做默认"。
4. workflow owner/editor 与 template 权限独立——workflow editor 不自动获得 template editor。
5. workflow 仅靠 scope / grants 控制访问；不在 workflow 层级引入"工作空间"概念。
6. workflow_input `type ∈ {material, asset, product, virtual_ip}`；slot.kind 必填且 resource_type 与 type 自洽；step_binding source 不得为 product / virtual_ip。
7. asset_kind 全局注册（无 scope）；slot.kind 引用 asset_kinds.name。
## 2026-09-17：发布时编译为 Conductor Definition

- 新增 `workflow_compilations`，记录源 Workflow version 到 Conductor name/version/hash 的不可变映射。
- 发布改为 `draft -> publishing -> published | publish_failed`；只有 registered compilation 可以 SubmitJob。
- P0 编译器使用拓扑波次，单节点发 SUB_WORKFLOW，多节点发动态并行子工作流和 JOIN。

## 2026-09-17：节点可配置单候选审批

- Step 新增 `approval_policy = when_multiple_candidates | always`。
- 多 Candidate 始终需要用户审批；always 进一步要求单 Candidate 也审批，不提供跳过多候选审批的策略。

## 2026-09-25：显式 Executable Workflow IR

- Workflow 发布采用 Semantic Compiler → `ExecutableWorkflowIR` → Conductor Backend Compiler 两阶段编译。
- IR 中节点统一称为 `ExecutableStepIR`；不再使用 `CompiledStepPlan`。
- Conductor Definition 内嵌每个 Step IR 降低后的执行语义，运行时不从 `workflow_compilations` 查询转换计划。
- 原 `StepBinding`/`input_bindings` 统一更名为 `SlotBinding`/`slot_bindings`，明确其目标是 Template Slot；Semantic Compiler 将其消解为 IR `context`。
