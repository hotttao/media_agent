# media_ai 历史生成链导入与继续执行 Plan

依据 [Spec](../spec/media-ai-history-migration.md)。本计划跨 `media_workflow`、`media_materials` 与对象存储；一条真实历史执行已导入并恢复成功，不能据此宣称全量迁移完成。按当前约定，UI 相关项目等待用户的新设计，本轮不实现。

## P1 · 源事实与排除清单

- [x] 从旧表关系重建 ModelImage → StyleImage → FirstFrame → Video → VideoPush；不把旧 `workflows.config` 当作生成链。
- [x] 输出完整 Video 分组和停在图片节点的未完成分组；逐记录排除真正断链项。
- [x] 用只读源库运行清点，并测试重复引用、缺失引用和合法阶段性成果。
- [x] 核对 Product/VirtualIp/pose/scene/movement 旧 ID 与目标素材映射，以及源媒体文件可取性。2026-10-03 最终只读复核：全部来源 ID 均有唯一目标映射；7 条纯文本 movement 的内容与目标记录一致，不需要 Object Path。完整纳入视频链上游后，需要 97 个 ModelImage、179 个 StyleImage、83 个 FirstFrame、125 个 Video、3 个 pose、4 个 scene、8 个 movement。547 个需读取的媒体字段中 546 个可读；1 条完整 VideoPush 的成片 URL 返回 404，故 182 个候选组中 181 个可完整摄取。旧 [核对记录](../human/research/007_media_ai_readiness.md) 的统计口径保留作历史记录。

## P2 · 历史资产摄取

- [x] 在 `media_materials` 建来源 ID 唯一约束、对象摘要与历史来源字段，提供迁移身份专用摄取/登记 API（默认禁用）。
- [x] 将一个 FirstFrame 谱系的真实图片搬到受控对象存储；校验摘要、类型、可读 URL 和独立于 Job 的复用。
- [x] 修正 Generated Asset 公开读取被内部写入中间件误拦截的问题；公开 Get/List 按企业与可见性过滤，按请求签发 URL。真实 API 对导入图片 GET/List 和 Range GET 已验证。
- [x] 测试同 ID 重试、摘要冲突、源 URL 不可达、缺失 Product/Virtual IP 映射与跨 Organization 拒绝；Workspace 级鉴权待 P3 导入 Job 接口验证。

## P3 · 导入未完成 Job

- [x] 在 `media_workflow` 增加 `awaiting_resume`、来源幂等键、历史 StepRun 来源字段及导入事务。
- [x] 实现迁移专用导入 API/CLI；按最深已完成图片节点冻结成功前缀和待运行后继。导入前验证 Workspace/企业/所有者归属；真实 API 样本已通过 P5 联调。
- [ ] Job 列表、详情和图上明确显示“待继续”、历史图片结果、下一节点与待补输入；普通 `SubmitJob` 不用于导入。

## P4 · 显式继续与 Conductor 恢复分支

- [x] 实现 Resume 授权、输入补齐、媒体复验、并发/幂等和 Outbox。
- [x] 编译可恢复分支并实现受控输出 restore；原成功节点不再派发 Capability，未完成节点沿现有 Worker Gateway 执行。
- [x] 单样本视频先摄取到目标对象存储并验证摘要/Range GET；fixture Worker 只返回受控结果，原生 Generated Asset 登记持久化 Object Path；真实执行已通过 P5。
- [x] Resume 冻结企业 Object Path 而非签名 URL；Materials 内部只读解析、Worker 领取时重签并校验过期/跨企业，覆盖延迟 Outbox 场景（单元测试、真实存储接口与完整执行均已验证）。
- [ ] UI 提供“继续执行”入口与缺失输入表单；页面显示正在执行节点、等待节点及结果。

## P5 · 联调与全量迁移门禁

- [x] 按 [E2E 规范](../../harness/contributing/e2e.md) 经统一网关/API Token 导入并继续**一条**停在 FirstFrame 的历史链；启动 `media_worker` fixture，验证只执行 video、旧图片不重复生成和结果媒体可访问。`TestHistoricalResumeSingleFixture` 已通过；fixture Worker 验收后停止，其他后端服务继续运行。
- [x] 按用户既有全量迁移授权执行批次，并按来源 ID 对账所有纳入/排除/失败项；121 个未完成组、125 个 Video、52 个 VideoPush 已导入并由公开 API 逐项核对，50 条断链排除，1 个缺成片 VideoPush 失败隔离。该异常待用户选择或来源文件恢复，不能声称所有记录完整迁移。
- [x] 验证断链样本被排除、合法中间图片保留、重复导入/Resume、跨账户拒绝、Conductor/对象存储暂时失败恢复。源清单排除 50 条；单元测试、隔离 PostgreSQL Outbox 测试及真实网关跨账户拒绝均通过。暂时故障通过注入失败并重试验证，不停止用户正在使用的服务。

## P6 · Text-only Movement

- [x] Resume 对 `movement` 支持空 URL、非空内容；校验冻结 Invocation 不引用 movement `asset_ref`。媒体型 movement 路径保持原校验；单元测试已通过。
- [x] 发布只将 FirstFrame 映射为视频 Capability 资源、将 movement 内容渲染进提示词的新 Template/Workflow 定义；新 Workflow `wf_793446b8-c078-4735-8a18-3b40a579f2f5@1` 已发布，视频 Template `tpl_995bf505-f75f-4b70-9934-f327ef3e5684@3` 的 Invocation 仅含 FirstFrame asset_ref，旧冻结定义不变。
- [x] 以无对象的现存 movement 做真实 API/Worker 验收：`job_f90ede43-8bd6-45a5-bc6a-445e2402f33d` 从 `awaiting_resume` 仅运行 video 到 `succeeded`，前三张历史图片 Asset ID 未变；Job movement 有内容、无 URL，Fixture Worker 完成 video。冻结 Invocation 不含 movement asset_ref，Outbox/Conductor 输入不需要伪 Object Path。此次所选来源 movement 内容是旧系统已有占位文本，仅验证纯文本输入契约，不代表提示词质量。

## P7 · 完成历史与全量写入

- [x] 为已有 Video 建受控、幂等的历史完成 Execution/StepRun API，不伪造 Conductor 运行；四资产与 Workspace 校验的单元测试已通过。
- [x] 完成单条 Video 的真实 API 验收；样本 VideoPush 的 2 条成员 Video 分别建立终态四节点 Job `job_6c2d2684-3ec8-4c65-acdc-e9f417edb804`、`job_3e7c1bda-76df-4b08-9c8d-87069ecbcb16`，共享上游图片按源 ID 复用，原视频媒体转存目标对象。全量逐 ID 对账仍在批处理门禁。
- [x] 建立独立 HistoricalVideoPush 聚合、迁移 API 与公开 Get/List；保留有序成员、发布字段和原时间，关联已完成 Video Job，不触发剪辑/发布。
- [x] 摄取 VideoPush 非空成片、缩略图和手工剪辑媒体为目标 Object Path；全量成功记录含 52 个成片、1 个缩略图、1 个手工剪辑对象，已通过源字段/API/存储摘要对账，空字段保持为空且旧 URL 未持久化。另 1 条来源成片缺失明确隔离。
- [x] 实现可重跑的全量批处理与来源 ID 对账：纳入、排除、失败、目标 ID、Object Path 和校验摘要；保留已导入 Job 原冻结 Workflow，接受续跑后的当前状态。已完成真实全量批次和 `--verify-report`：298 个目标记录、50 个排除项及 538 个目标对象的实际可读性/存储摘要均核对通过；唯一来源缺失项保持隔离，源库只读。

风险：源模板未必能匹配所有旧产物；导入成功前缀保留真实来源而不冒充当前 Template Version 的历史运行。7 个 movement 已按纯文本契约处理并完成一条真实 API/Fixture Worker 续跑。VideoPush `d8ef3ac8-32c9-414b-bc5d-1c1eeadca214` 的来源成片返回 404，本地 `/data` 未找到同名文件；成员 Video 已导入，该发布记录仍隔离。样本 mock 成功不等于生产 Provider 已验证。

## 迁移能力覆盖矩阵（样本门禁）

| 源能力/事实 | 目标能力 | 当前验证 | 全量门禁 |
|:---|:---|:---|:---|
| Product、VirtualIp、pose、scene、movement 映射 | 目标 Organization 内素材与 JobInput | 样本 Product/VIP 及 movement 对象已定位；其余映射待清点 | 每条纳入组可解析或列明隔离原因 |
| ModelImage、StyleImage、FirstFrame | Imported Generated Asset + 成功 StepRun | 一条 FirstFrame 谱系已通过 API 导入 | 按源 ID 去重、摘要与时间对账 |
| 无视频的完整图片谱系 | `awaiting_resume` Job | 一条真实 Job 已导入且 Resume 后 succeeded，原三个 Asset ID 未变 | 只执行缺失后缀、重复命令幂等 |
| Video | 每条源 Video 一个终态 Job + 四个历史 StepRun | 125 条真实 Video 已导入；API 逐项核对来源、原时间、四节点结果与对象摘要 | 已通过 |
| VideoPush | 独立 HistoricalVideoPush 聚合，关联有序成员 Video Job | 52 条导入并逐项核对原字段/时间/有序成员/媒体，HTTP 和 CLI Get/List 验收通过 | 1 条来源成片 404 单独隔离，待处理选择 |
| 断链记录 | 排除报告 | CLI 已输出排除原因 | 不创建目标 Job/Asset，逐条对账 |
