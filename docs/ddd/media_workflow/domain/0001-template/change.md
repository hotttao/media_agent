# template domain 变更记录

## 2026-07-03 · 初始建模（media_agent v1）
由 `media_ai` 的 `prompt_template` 设计迁入并扩展：

- 旧 prompt_template 用 `input_schema.variables[]`（从 `text/template` 解析）与 `output_type`（单值字符串）；本版改为 inputs[]/outputs[] 显式声明型 slot（`Template Slot`），同时放弃 Go `text/template` 强语义，改成 `{slot_name}` 占位符。
- Template 表新增 `scope` 字段，强约束 1:1 且不可改；同步引入 `template_slots` 表把 inputs/outputs 落到关系表里。
- `status` 由旧版 draft/published/archived 一致沿用；update 自增 version；旧 version 不可改。

## 修改记录

### 2026-09-25 · Invocation Mapping 与通用模板渲染任务

- Template Version 新增不可变 `invocation_mapping`，显式把业务 Slot 转换为 Capability 固定参数。
- 初始表达式限定为 `render_template`、`asset_ref(slot)` 与有序 `list(...)`，不允许任意脚本。
- 模板渲染使用 `media_workflow` 拥有的内部通用任务 `media_render_template_v1`，固定契约为 `template + context map -> prompt`；它不是 Execution Capability。

### 2026-07-21 · 补 Publish 能力 + archive 边界 + 依赖修正
- 新增 `PublishTemplate`（`POST /api/v1/templates/{id}/publish`，draft -> published，owner），补全状态机 `draft ↔ published -> archived` 中 published 的产生路径（原状态机有 published 态但无操作可达）。
- 校验细则第 7 条补 `PublishTemplate` / `ArchiveTemplate` 在状态流转中的角色；新增第 9 条 archive 边界：archive 被运行中 Job 引用的 template 不中断 Job（Job 持 `template_version` 快照），新 workflow 拒绝引用（workflow_validation 闸2），offline scan 待后续。
- `dependencies` 去掉 `scope`（已降为 shared），补 `media_materials.asset_kind`（跨服务 `BatchGetAssetKind` 校验 slot.kind）。
- 关联术语 `Resource Type` 链接修正：原指向 media_workflow CONTEXT（无此词条，断链），改为 media_materials CONTEXT。

### 2026-07-03 · 第二轮反馈：允许 const 满足 required input
- 影响：Render 接口的 ResolvedSlot 增加 `SlotSource = asset|const` 字段。`asset` 由 `media_workflow.workflow_execution` 解析 binding 后通过 `AssetRead` 填充；`const` 由 step binding.const_value 直接填入。
- 渲染规则：source 分两路，prompt_body 中 `{name}` 都能被替换；`{name.key}` 在 source=const 时不支持。

### 2026-07-03 · 第三轮反馈：template 作为资源级 ACL 实体
- 加 `visibility ∈ {private, team, public}` 与 `template_grants (template_id, team_id, permission)`，与 skill / virtual_ip / workflow 同模型。
- 加 GrantTemplate / RevokeTemplate / ChangeVisibility 三个 endpoint；Delete 限 owner。
- 校验时机：每次 service 方法入口先 `LoadMyPermission(user, template)`，不足 403。

### 2026-07-03 · Review 反馈：output 改回单一（反转第一轮多输出决策）
- 第一轮 Q&A 曾定 `outputs[]` 为多输出列表；用户 review 时纠正"Template outputs 是单一输出，不会有多个输出"，本节反转该决策。
- 改动：`outputs []Slot` → 单一 `output_resource_type` 字段（提升到 `templates` 表）；`template_slots` 表去掉 `kind` 列并更名为 `template_inputs`（只存输入槽位）；Render 返回 `OutputType string`（原 `OutputTypes []string`）；StepRun 产物 `output_asset_ids` → `output_asset_id`（单值）。
- 连带：`source=step_output` 的 SlotBinding **不需要**指定前驱 step 的哪个 output（曾因此考虑加 `ref_step_output_name`，现已废弃）——前驱 step 恒定产出单一 asset，`ref_step_key` 足够。

### 2026-07-03 · Review 反馈：全面对齐 media_ai 的 asset_kind 模型
- input slot 改为单一 `kind` 字段（asset_kind name），去掉 `resource_type` + `asset_kind` 两字段；media_type / resource_type / 落表均由 kind 推导。
- output 改为 `output_asset_kind`（asset_kind name，resource_type=ASSET），取代 `output_resource_type`。修正 P0-4（产物 asset_kind 由 template 定义时设置）。
- Template 新增 `platform` / `gen_way` / `gen_type` 生成路由字段，供 workflow_execution 分发下游生成器。

## 2026-09-17：以版本化 Execution Capability 取代自由路由字符串

- Template 新版本改为引用 `(capability_key, capability_version)`。
- `platform` / `gen_way` / `gen_type` 只保留为旧数据迁移字段，新写路径不再使用。
- Capability 的输入 schema、输出类别与 Conductor Task Definition 在发布时校验。
- Render 返回 `OutputAssetKind` + `OutputMediaType`（从 asset_kind.media_type 推导），取代 `OutputType`。
- 校验：input slot.kind 与 output_asset_kind 必须在 asset_kind 注册表已注册且未停用（调 `BatchGetAssetKind`）。

## 来自业务规则的强约束
1. `Scope` 不可变（不允许 update 时跨 scope）。原因：跨 scope 引用会让旧 version 失效，破坏可重现性。
2. `Inputs` 内部 `Name` 唯一。
3. `Required` 仅 input 槽位生效。
4. Template 是 owner-creator 制（不是 team-owned）。一个 user 可拥有多个 Template，团队共享走 grants 路径。
5. `visibility=public` 也受 `scope` 业务线隔离——任何 user 跨 scope 看到 public template 不会被拒（read 不拒），但若把 public template 绑到另一 scope 的 workflow，则 workflow_validation 拒 `SCOPE_MISMATCH`。
6. **单一 output**：一个 Template 恒定产出**一个** generated_asset，类别由 `output_asset_kind` 决定；不支持多输出。
7. input slot.kind / output_asset_kind 引用全局 asset_kind 注册表（无 scope）；kind 决定 media_type / resource_type / 落表。
