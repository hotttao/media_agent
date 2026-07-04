# media_workflow + media_materials 落地计划

> 基于 `docs/spec/media_workflow/impl.md` + `docs/spec/media_materials/impl.md`。把两份 spec 切成可执行阶段，标注依赖、风险、验收标准。

## 总览

两个新服务：
- **media_materials**：asset_kind 注册表 + material / generated_asset / product / virtual_ip + material_bindings。
- **media_workflow**：template / workflow / job，pull 模式 execution + NEED_CONFIRM + SSE。

依赖方向：media_workflow → media_materials（仅 BatchGetAssetKind + UpsertGeneratedAsset 两个 RPC）。

## 阶段划分

```
P1 media_materials 基础  ──→ P2 media_workflow Template+Workflow（闸1）──→ P3 media_workflow Job（闸2+pull+confirm+SSE）──→ P4 集成 + e2e
```

每阶段独立可验收；P2 依赖 P1 的 BatchGetAssetKind；P3 依赖 P2 的 workflow/input_schema；P4 依赖 P3。

---

## P1 · media_materials 基础

**目标**：materials 服务能跑起来，asset_kind 注册表 + 五类资源 CRUD + 跨域 RPC 就绪。

### Checklist

- [ ] 服务骨架：`make -C media_materials hz new` 生成（复用 harness new_server）；go.mod 接入 hertz_infra。
- [ ] DB 迁移 `media_materials/migrations/000001_init.up.sql`：asset_kinds / materials / generated_assets / material_bindings / products / virtual_ips / virtual_ip_grants（按 spec §3）。
- [ ] asset_kind seed 迁移 `000002_asset_kind_seed.up.sql`：SYSTEM kinds（scene/pose/movement/music/caption/model_image/style_image/first_frame/ai_video/product_image/product/virtual_ip/...）。
- [ ] IDL：`idl/media_materials/{asset_kind,material,generated_asset,product,virtual_ip}.proto`（按 spec §2）；`make hz-model` + `make -C media_materials hz-update`。
- [ ] domain 层：asset_kind / material / generated_asset / product / virtual_ip 实体 + repository 接口。
- [ ] dal 层：GORM model + repo 实现 + GORM Gen。
- [ ] service 层：CRUD + ListByOwner/ListBySource；visibility ACL（material/generated_asset/virtual_ip 的 MyPermission 推导）；asset_kind 校验（resource_type/media_type/status）；material_bindings 同 scope 校验。
- [ ] handler 层：薄 handler 绑定→command→service→响应。
- [ ] 内部 RPC（不暴露 HTTP）：`AssetKindService.BatchGetAssetKind`、`GeneratedAssetService.UpsertGeneratedAsset`。
- [ ] config：`supported_scopes` 加载。
- [ ] arch-check + gofmt + go test 通过。

### 验收
- RegisterAssetKind / CRUD 各资源 API 可调通。
- BatchGetAssetKind 返回 resource_type/media_type/status。
- UpsertGeneratedAsset 落 generated_asset + 生成记录字段。
- material_bindings 跨 scope INSERT 被拒。
- visibility：private material 非 owner 不可见。

---

## P2 · media_workflow Template + Workflow（闸 1）

**目标**：template / workflow 的 CRUD + 资源 ACL + ValidateWorkflow（含 input_schema 派生快照）。

### Checklist

- [ ] DB 迁移 `media_workflow/migrations/000001_init.up.sql`：templates / template_inputs / template_versions / template_grants / workflows / workflow_steps / step_bindings / workflow_grants（按 spec §3.1–3.2）。
- [ ] IDL：`idl/media_workflow/{template,workflow}.proto`；hz 生成。
- [ ] domain：template / workflow 实体 + repository；template 版本化（update 追加 template_versions）。
- [ ] service：Template CRUD + grants + visibility + Archive；Workflow CRUD + grants + visibility + Archive。
- [ ] **ValidateWorkflow（闸 1）**：DAG 结构（A）+ slot/binding（B）+ input_schema 派生快照（C）+ template 引用 & 跨资源 ACL（D）+ scope（E）+ asset_kind 注册（调 media_materials.BatchGetAssetKind）。
- [ ] CreateWorkflow/UpdateWorkflow 内联闸 1；ValidateWorkflow RPC 不写库返回 ValidationResult。
- [ ] handler + 路由。
- [ ] arch-check + gofmt + go test。

### 验收
- 创建 template（slots + output_asset_kind + platform + needs_confirm + candidate_count）。
- 创建 workflow，input_schema 自动派生快照。
- 跨 scope 引用 template 被拒（SCOPEMismatch）；archived template 被拒；caller 无 viewer+ 被拒（TEMPLATE_INACCESSIBLE）。
- 多 sink / 离群 / 后向 ref 被 DAG_INVALID 拒。
- template update 后旧 version 可查（GetTemplateVersion）。

---

## P3 · media_workflow Job（闸 2 + pull + confirm + SSE）

**目标**：SubmitJob + ClaimStepRun/CompleteStepRun/ConfirmStepRun/RegenerateStepRun/FailStepRun + DAG 推进 + SSE + Retry。

### Checklist

- [ ] DB 迁移 `media_workflow/migrations/000002_job.up.sql`：jobs / step_runs（按 spec §3.3，含 candidates/selected_index/claimed_by/claimed_at）。
- [ ] IDL：`idl/media_workflow/job.proto`（按 spec §2.3，含 JobInput bundle + Candidate + JobEvent + 全部 RPC）；hz 生成。
- [ ] domain：job / step_run 实体 + 状态机（BLOCKED→PENDING→CLAIMED→NEED_CONFIRM→SUCCEEDED/FAILED）。
- [ ] **SubmitJob（闸 2）**：bundle ↔ input_schema 校验（required 覆盖 / 同类唯一 / 无多余）；创建 Job + StepRuns（root=PENDING，非 root=BLOCKED）。
- [ ] **ClaimStepRun**：按 platform/gen_way/gen_type 找 PENDING StepRun；加载 bundle + template snapshot；按 binding.source 解析 slot；渲染 prompt；返回 rendered_prompt + input_assets + candidate_count；StepRun→CLAIMED。
- [ ] **CompleteStepRun**：needs_confirm=true→NEED_CONFIRM（存 candidates，Job→NEED_CONFIRM，SSE ET_STEP_NEED_CONFIRM）；false→取 primary，UpsertGeneratedAsset，SUCCEEDED，推进 DAG。
- [ ] **ConfirmStepRun**：选定 candidate→UpsertGeneratedAsset→SUCCEEDED→推进 DAG。
- [ ] **RegenerateStepRun**：NEED_CONFIRM→PENDING（重拉）。
- [ ] **FailStepRun**：CLAIMED→FAILED；Job→FAILED。
- [ ] **CancelJob** / **RetryJob**（failed→重置 failed/skipped/need_confirm，retry_count+1，retry_history 追加，input_override）。
- [ ] **DAG 推进**：StepRun SUCCEEDED 时下游 BLOCKED→PENDING；sink SUCCEEDED→Job SUCCEEDED。
- [ ] **Claim 超时扫描**：后台 goroutine 扫 `claimed AND claimed_at<now()-claim_timeout`→PENDING。
- [ ] **SSE StreamJobEvents**：`GET /api/v1/jobs/:id/events`；JobEvent 单调 version；Last-Event-ID 补推窗口。
- [ ] handler + 路由（worker RPC 不做认证）。
- [ ] arch-check + gofmt + go test。

### 验收
- SubmitJob 拒绝缺 required / 同类重复 / 多余类型。
- worker ClaimStepRun 拿到 rendered_prompt + input_assets；无任务时 404。
- needs_confirm step：CompleteStepRun→NEED_CONFIRM；ConfirmStepRun→SUCCEEDED 推进下游；RegenerateStepRun→重拉。
- 非 confirm step：CompleteStepRun→直接 SUCCEEDED。
- DAG 多步：前驱 SUCCEEDED 后下游自动 PENDING；sink SUCCEEDED→Job SUCCEEDED。
- CLAIMED 超时→自动 PENDING。
- SSE 推 status 变更 + needs_confirm 事件。
- RetryJob 重置失败 step。

---

## P4 · 集成 + e2e

**目标**：media_workflow ↔ media_materials 联通；编排层组装 bundle；端到端跑通。

### Checklist

- [ ] media_workflow 调 media_materials.BatchGetAssetKind（闸 1）—— hertz client + hertz_infra/clienthertz 治理。
- [ ] media_workflow 调 media_materials.UpsertGeneratedAsset（StepRun SUCCEEDED 时）。
- [ ] 编排层/BFF（或在 media-cli 里）：给定 product_id/virtual_ip_id，调 materials ListByOwner/ListBySource 组装 JobInput bundle，调 SubmitJob。
- [ ] worker stub（模拟 browser extension）：轮询 ClaimStepRun → 返回假候选 url → CompleteStepRun；用于 e2e。
- [ ] e2e：建 asset_kind seed → 建 material/product/virtual_ip → 建 template（needs_confirm=true, candidate_count=4）→ 建 workflow → 编排层组装 bundle → SubmitJob → worker claim+complete（4 候选）→ ConfirmStepRun 选 1 → sink SUCCEEDED → generated_asset 落 materials。
- [ ] e2e：multi-step DAG（model_image→style_image→first_frame→ai_video）全链路。
- [ ] e2e：retry / regenerate / claim 超时 / cancel 场景。
- [ ] docker-compose 加 media_materials + media_workflow + postgres。

### 验收
- 端到端：用户提交 job → worker 生成 → 用户确认 → 产物落 generated_assets，全程 media_workflow 不直读 materials（除 BatchGetAssetKind + UpsertGeneratedAsset）。
- e2e 全绿。

---

## 风险与对策

| 风险 | 对策 |
|:---|:---|
| NEED_CONFIRM 状态机复杂（claim/confirm/regenerate/timeout 交织） | P3 先写状态机单测覆盖所有转移，再联调；用状态图固化 |
| SSE 长连接在 Hertz 下的稳定性 | 参考 media-scheduler 的 SSE 实现；Last-Event-ID + 补推窗口兜底 |
| input_schema 派生与快照一致性 | CreateWorkflow/UpdateWorkflow 在事务内派生+落库；ValidateWorkflow 派生结果与快照对比测试 |
| cross-service id（generated_assets.job_id 指向 media_workflow）无 FK | 文档约束 + service 层校验；不依赖 DB FK |
| bundle 自包含导致 caller 组装逻辑散落 | 编排层/BFF 收口组装；media-cli 提供组装命令复用 |
| asset_kind 注册表与 scope 解耦后跨 scope 引用 | 闸 1 强制 workflow.scope==template.scope；slot.name 注册表校验 |

## 依赖

- P1 无外部依赖（除 media_auth 的 user/team 概念，本期 media_materials 不直接调 media_auth，visibility 用 team_id 字段预留）。
- P2 依赖 P1 的 BatchGetAssetKind。
- P3 依赖 P2 的 workflow + input_schema。
- P4 依赖 P3 + P1。

## 开发顺序建议

P1 → P2 → P3 → P4。每阶段 PR 独立 review。P3 可拆两个 PR：(a) SubmitJob + Claim + Complete + DAG 推进；(b) NEED_CONFIRM + SSE + Retry。
