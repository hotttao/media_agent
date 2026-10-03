# media_ai 历史生成链迁移前只读核对

2026-10-02 对旧 MySQL、旧媒体服务和测试企业 `8afeb26b9df643952f4f816576eb2504` 的目标公开 API 执行 `media_ai_history_migrate --audit-readiness`。未写入旧库或目标库，也未导入其他 Job。脚本和复跑方法见 `media_workflow/tools/media_ai_history_migrate/README.md`。

| 核对项 | 需要 | 找到对应记录 | 来源媒体可读取 |
|:---|---:|---:|---:|
| Product | 97 | 97 | 本工具不重复探测已独立迁移的商品图片 |
| Virtual IP | 1 | 1（按唯一名称匹配） | 本工具不重复探测已独立迁移的形象图片 |
| pose / scene | 2 / 4 | 2 / 4 | 6 / 6 |
| movement | 8 | 8 | 1 / 8 |
| ModelImage / StyleImage / FirstFrame | 97 / 119 / 83 | 来源记录均存在 | 299 / 299 |
| Video | 125 | 来源记录均存在 | 125 / 125 |

合计探测了 438 个候选输入/产物媒体引用，431 个 HTTP Range GET 取得了实际数据。另 7 个 movement 在源库的 `url` 为空，目标 Material 虽有来源 ID 和文本内容，但 `object_path` 也为空：

`178b8b1e-1b5b-43f2-9bab-172e196607af`、`3097d950-768f-4e10-9e6d-d3a6adf15e51`、`5f100242-7a2a-4761-9dd3-8f844c7d2992`、`6a9e92fa-9e22-4dff-8484-e12d57806bf2`、`756d3c36-5391-4d2c-8c81-987162223c2b`、`cf5f9900-80ca-4526-932f-564a63f35793`、`ebcc3490-110f-46e2-adc4-f41c4b2c7aaa`。

182 个候选分组中，123 个关联的来源媒体齐全，59 个引用上述 7 个无视频文件的 movement。这里的“媒体齐全”不是“已迁移”。现按纯文本动作处理这 7 条：其内容作为提示词输入，不要求视频对象，也不伪造 Object Path；59 个分组不能仅凭 movement URL 缺失判为断链。仍须以新发布的 Template/Workflow Version 做真实 API 验收；旧冻结版本若把 movement 映射为 `asset_ref`，Resume 会拒绝，不能隐式改变旧版本。已经完成的视频产物本身均可读取。

另有 50 条真正断链或无视频任务记录已按源关系列入排除清单，不在 182 个候选分组内。异常回归覆盖：断链排除与合法图片中间结果保留、重复导入与 Resume 幂等、真实网关跨账户 GET 404 / Resume 403、存储暂时失败时 Job 保持待续及 Worker 不交付无效输入、Conductor 失败后 Outbox 释放并在隔离 PostgreSQL 中重新领取。没有为测试停止运行中的 Conductor 或对象存储。

## 2026-10-03 修正口径与复核

上面的 438/431 和 123/59 是旧审计口径：它把纯文本 movement 的空 URL 当成缺失媒体，且未检查 VideoPush 自身媒体。新审计逐条比较 7 个纯文本 movement 的来源/目标内容，允许双方无 Object Path；只对实际存在 URL 的字段发起 Range GET，同时探测完整 VideoPush 的成片、缩略图和手工剪辑媒体。

重新只读核对：Product 97/97、Virtual IP 1/1、pose 2/2、scene 4/4、movement 8/8 均可唯一映射；486 个实际媒体字段中 485 个可读。182 个候选组中 181 个媒体齐全。唯一阻塞的是 VideoPush `d8ef3ac8-32c9-414b-bc5d-1c1eeadca214` 的来源成片，`/uploads/teams/cf88f650-a530-4356-81db-391b62522778/clips/2026-05-11/clip-de7746c3442bd0ad84deec1ad0f52d3e_luxury.mp4` 返回 HTTP 404；本机 `/data` 未找到同名文件。其成员 Video 仍可独立迁移，但该发布记录不得标为完整迁移，也不能伪造媒体或悄悄丢弃非空来源字段。

纯文本动作的目标 Workflow `wf_793446b8-c078-4735-8a18-3b40a579f2f5@1` 已发布，并通过一个真实 Job/Fixture Worker 续跑验收；这证明技术契约可用，不意味着旧占位文本有可用的业务提示词质量。另一个真实 VideoPush 样本已导入目标独立历史发布记录并核对字段、成员顺序与成片媒体；全量写入尚未执行。

## 2026-10-03 全量批次与最终对账

最终预检补全 Video 分组引用的上游图片与 pose：ModelImage 97、StyleImage 179、FirstFrame 83、Video 125、pose 3、scene 4、movement 8 均有来源及目标映射；547 个实际媒体字段中 546 个可读。前一节 486 个探测字段少计了视频链独有的 60 个 StyleImage 和 1 个 pose，已修正工具，历史数字保留说明。

批量迁移通过 API 将 121 个图片中间组登记为历史 Job、125 条 Video 登记为终态四节点 Job、52 条 VideoPush 登记为独立历史发布记录。50 条断链记录排除，来源成片 404 的 1 条 VideoPush 隔离，其成员视频已迁入。目标 Workspace 为 `2e6c5e63-b343-47c4-9f4e-64eb3c7855ce`，企业和账号归属与既有测试数据一致。已有且已续跑的两条历史 Job 保留原冻结 Workflow 与当前执行状态。

私有对账文件位于 `media_workflow/.migration_reports/full-retry-2026-10-03.jsonl`（0600，Git 忽略）。`--verify-report` 只读核对完整源记录集合及目标公开 API：298 个成功目标记录的来源、归属、历史 StepRun 资产/原时间、发布文字/标志/原时间和有序成员均匹配；538 个唯一目标对象可读取且存储 SHA-256 与报告一致。发布媒体包括 52 个成片、1 个缩略图和 1 个手工剪辑对象。命令因仍有 1 个隔离项返回非零，不能将其当作“所有记录完整迁移”。

`media-cli/e2e/TestHistoricalPublicationGateway` 通过，包含实际下载样本成片并重算 SHA-256、成员终态 Job、无凭据拒绝及 Workspace 隔离；CLI `media_workflow historical-publication get/list` 实际执行通过。UI 未修改。源 MySQL 全程只读。

补齐公开 Job/StepRun 响应中的执行时间与失败原因映射后，重新部署并再次通过上述完整对账。迁移临时凭据已撤除，两个内部导入 API 均恢复 401 拒绝；后端保持运行，5173 返回 200。`media_workflow` 全量 Go 测试、架构检查及真实网关发布记录 E2E 均通过。
