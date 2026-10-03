# media_ai 历史生成链导入与继续执行 Plan

依据 [Spec](../spec/media-ai-history-migration.md)。本计划跨 `media_workflow`、`media_materials` 与对象存储；一条真实历史执行已导入并恢复成功，不能据此宣称全量迁移完成。按当前约定，UI 相关项目等待用户的新设计，本轮不实现。

## P1 · 源事实与排除清单

- [x] 从旧表关系重建 ModelImage → StyleImage → FirstFrame → Video → VideoPush；不把旧 `workflows.config` 当作生成链。
- [x] 输出完整 Video 分组和停在图片节点的未完成分组；逐记录排除真正断链项。
- [x] 用只读源库运行清点，并测试重复引用、缺失引用和合法阶段性成果。
- [x] 核对 Product/VirtualIp/pose/scene/movement 旧 ID 与目标素材映射，以及源媒体文件可取性。只读全量审计见 [核对记录](../human/research/007_media_ai_readiness.md)：来源 ID 均可映射；438 个媒体引用中 7 个 movement 源 URL 与目标 Object Path 均为空，影响 59 个分组的媒体完整性，须在全量门禁中单独处理。

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
- [ ] 用户确认样本后才执行全量导入；按来源 ID 对账所有纳入/排除/失败项，可重跑且不污染源库。
- [x] 验证断链样本被排除、合法中间图片保留、重复导入/Resume、跨账户拒绝、Conductor/对象存储暂时失败恢复。源清单排除 50 条；单元测试、隔离 PostgreSQL Outbox 测试及真实网关跨账户拒绝均通过。暂时故障通过注入失败并重试验证，不停止用户正在使用的服务。

## P6 · Text-only Movement

- [x] Resume 对 `movement` 支持空 URL、非空内容；校验冻结 Invocation 不引用 movement `asset_ref`。媒体型 movement 路径保持原校验；单元测试已通过。
- [ ] 发布只将 FirstFrame 映射为视频 Capability 资源、将 movement 内容渲染进提示词的新 Template/Workflow Version；旧版本保持不可变。
- [ ] 以无对象的现存 movement 做 API 验收，确认 Job/Outbox 无伪 Object Path，Worker 收到的资源数组不含空引用。

## P7 · 完成历史与全量写入

- [x] 为已有 Video 建受控、幂等的历史完成 Execution/StepRun API，不伪造 Conductor 运行；四资产与 Workspace 校验的单元测试已通过。
- [ ] 完成每条 Video 的真实 API 验收；CLI 单条导入、来源 ID/关系校验、图片/视频对象按源 ID 摄取及 256 MiB 有界上传代码和单元测试已完成，仍需真实数据验证摘要与时间对账。
- [x] 建立独立 HistoricalVideoPush 聚合、迁移 API 与公开 Get/List；保留有序成员、发布字段和原时间，关联已完成 Video Job，不触发剪辑/发布。
- [ ] 摄取 VideoPush 非空成片、缩略图和手工剪辑媒体为目标 Object Path；逐字段校验摘要，空字段保持空值，不留旧系统 URL（单条命令与单测已实现，待真实来源媒体和目标 API 验证）。
- [ ] 实现可重跑的全量批处理与来源 ID 对账：纳入、排除、失败、目标 ID、Object Path 和校验摘要。批处理、唯一映射、逐条私有报告已实现；媒体路径/摘要对账和真实 API 验收未完成。仅在能力与测试全部通过后按用户授权执行，不对旧库写入。

风险：源模板未必能匹配所有旧产物；导入成功前缀应保留真实来源而不是冒充当前 Template Version 的历史运行。7 个 movement 已按纯文本契约处理，但真实 API 验收和新 Workflow 发布未完成；尚未执行全量导入。样本的 mock 成功不等于生产 Provider 已验证。

## 迁移能力覆盖矩阵（样本门禁）

| 源能力/事实 | 目标能力 | 当前验证 | 全量门禁 |
|:---|:---|:---|:---|
| Product、VirtualIp、pose、scene、movement 映射 | 目标 Organization 内素材与 JobInput | 样本 Product/VIP 及 movement 对象已定位；其余映射待清点 | 每条纳入组可解析或列明隔离原因 |
| ModelImage、StyleImage、FirstFrame | Imported Generated Asset + 成功 StepRun | 一条 FirstFrame 谱系已通过 API 导入 | 按源 ID 去重、摘要与时间对账 |
| 无视频的完整图片谱系 | `awaiting_resume` Job | 一条真实 Job 已导入且 Resume 后 succeeded，原三个 Asset ID 未变 | 只执行缺失后缀、重复命令幂等 |
| Video | 每条源 Video 一个终态 Job + 四个历史 StepRun | 终态 API 单元测试通过；真实源 Video 未导入 | 视频媒体摄取、真实 API 验收和逐源 ID 对账 |
| VideoPush | 独立 HistoricalVideoPush 聚合，关联有序成员 Video Job | 聚合、接口与单条迁移命令的单元测试通过，真实 API 对账未完成 | 原字段/成员/媒体全部对账，不触发剪辑/发布 |
| 断链记录 | 排除报告 | CLI 已输出排除原因 | 不创建目标 Job/Asset，逐条对账 |
