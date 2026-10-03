# media_ai 历史生成链导入与继续执行 Spec

## 范围与现状

源事实是 `Product/VirtualIp -> ModelImage -> StyleImage -> FirstFrame -> Video[] -> VideoPush` 的关联，不是旧 `workflows.config`。本文约束 `media_workflow`、`media_materials` 和对象存储的迁移接口。**本轮迁移纳入素材、图片链、单条 Video、未完成执行及 VideoPush 的独立历史记录；不实现多视频剪辑或重新发布流程。**只读源清单、单组图片摄取、历史 Generated Asset 登记、未完成 Job 导入、显式 Resume 与 Outbox 均已实现；仅一条真实历史链完成了端到端恢复验收，不能声称具备全量迁移能力。用户已允许在所有功能完备后执行全量迁移；功能门禁未满足前不得启动全量写入。旧库全程只读。UI 相关工作按用户要求暂停，等待新的设计。

## 源清单与纳入规则

- `media_workflow/tools/media_ai_history_migrate` 输出源节点、逐 Video 谱系、`lineage_groups`、`excluded` 和 `issues`。只有上游引用存在且 Product/VirtualIp 一致的组可进入导入器；同一图片可被多个分支引用，按源 ID 只摄取一次。
- 有 Video 的完整组按每条 Video 源 ID 导入终态 Execution，并保留各自原时间；完整 VideoPush 另存有序成员与目标 Job 关联、原发布字段和已摄取媒体。未匹配 `WorkflowTemplate` 不视作断链，也不猜测模板版本。
- 没有下游的 ModelImage、StyleImage、FirstFrame 分别形成停在该节点的未完成执行候选；对应 `next_step_key=style_image/first_frame/video`。上游不存在或错配、无 Video 的 VideoTask、引用不存在 Video 的 VideoPush 排除；记录源 ID 和原因。未完成执行不因没有下游而排除。
- 抽样 `--only-task` 是 `sample_projection`，不能推断一个 VideoPush 完整或直接执行全量导入。

## 目标事实与接口

`media_materials` 增加受控的历史 Generated Asset 摄取/登记能力：`(source_system, source_record_id)` 唯一，保存 source SHA-256、受控 Object Path、Asset Kind、Organization/Scope、原所有者、原创建时间和导入时间；摄取拒绝任意外部 URL 作为最终资产引用。`origin=native` 仍要求 Job/StepRun/Template；`origin=legacy_import` 以来源键/摘要代替这些生成记录字段，供多个导入 Job 的 StepRun 复用。重复 ID 同摘要返回原 Asset；同 ID 不同摘要报冲突。旧 Product/VirtualIp/pose/scene/movement 先通过现有素材映射表解析，缺失映射则隔离，不创建半成品 Job。

`media_workflow` 增加迁移专用 `ImportIncompleteExecution`（仅可信迁移身份）与用户 `ResumeExecution`。导入请求包含 source group ID、旧节点 ID/时间、目标所有者和 Workspace、所选已发布 Workflow 版本、已登记 Asset ID、已知 Workflow Input 以及缺失输入名；不接收浏览器伪造的成功 StepRun。目标 `jobs` 增加 `origin=native|legacy_import`、来源幂等键、`imported_at` 和仅用于历史导入的 `awaiting_resume` 状态；`step_runs` 增加历史来源 ID/原时间/资产摘要，已完成前缀为 succeeded、后继为 blocked，且无 Conductor ID。导入和资产登记以来源键幂等，失败重试不重复生成。

`POST /api/internal/v1/jobs/import-completed` 使用同一迁移身份与请求结构，但 `source_group_id=video:<source_video_id>` 且 `completed` 必须依序包含 `model_image`、`style_image`、`first_frame`、`video` 四个已登记资产。服务复核来源 ID、归属、Workspace、已发布四节点 Workflow、各资产类别与校验摘要，在单个事务中保存终态 `succeeded` Job 和四个历史成功 StepRun；原视频时间为 `finished_at`，导入时间为 `imported_at`，无 Conductor ID。按源 Video ID 和请求摘要幂等，同 ID 不同事实冲突。共享上游资产只登记一次，可被多个终态 Job 引用。该接口只表达单条 Video 的生成执行；VideoPush 不得合并为一个单输出 Step。

`HistoricalVideoPush` 是独立历史发布聚合，不修改成员 Job。迁移接口 `POST /api/internal/v1/historical-video-pushes/import` 仅接受迁移身份，请求携带源 Push ID、目标 Workspace/Owner/Organization、原 Product/Virtual IP/Scene/Template、标题/正文/状态/资格/发布标记、原创建/修改时间、有序成员 `(source_video_id, job_id)`，以及非空 `url/thumbnail/manual_clip_url` 对应的目标 Object Path 与 SHA-256。服务验证每个 Job 是同 Workspace/Owner/Organization 的 `legacy_import` 终态 Video Job，来源 ID 匹配；所有媒体路径属于本企业目标存储且摘要存在。单事务保存记录与成员，`(workspace, media_ai, source_push_id)` 唯一，同事实重试返回原记录，不同事实 409。公开 Get/List 只允许有权访问 Workspace 的用户并对媒体路径按请求重新签发 URL。迁移工具先摄取非空 Push 媒体，再导入记录；空字段保持空值，绝不持久化旧服务 URL。该记录不触发剪辑、发布或 Conductor。

迁移 CLI 的 `--import-video <source_video_id>` 只能选择源清单中上游完整、未排除的 Video；复核 VideoTask、Product/VirtualIP、Model/Style/FirstFrame 亲子关系及目标输入映射。依次按源 ID 摄取三张图片和视频，收到四个 Imported Generated Asset ID 后再调用终态 Job API。图片上限 16 MiB，视频上限 256 MiB；源端和目标上传服务均有界落盘、流式上传并校验 SHA-256，超限不写 Job。已摄取部分资产在重试时按来源键复用。

导入写入前，`media_workflow` 必须以内部服务凭证向 `media_control` 查询目标 Workspace，并以原所有者作为 acting user。查询结果的 Organization 必须与导入请求一致；Workspace 不存在、无权访问或控制服务不可用均拒绝导入，不能只信任迁移请求中的 Workspace ID。

`ResumeExecution(job_id, input_patch, idempotency_key)` 由当前 User 调用，授权 Job 所属 Workspace，校验待执行节点所需的 pose/scene/movement 等输入、素材可达性、媒体对象与冻结 Workflow Version；缺失则保留 `awaiting_resume` 并返回字段级错误。成功时事务内冻结补齐输入、置 Job 为 running、写 StartResume Outbox；重复 key 返回原执行，其他并发 Resume 返回冲突。恢复不可覆写已成功的历史 StepRun。

对于来源 `movement_materials.url` 为空而 `content` 非空的记录，目标 Material 按 Text-only Movement 保留内容并允许无 Object Path。Resume 的 `movement` 输入可携带空 URL 与非空内容，但仅在冻结 Template Invocation 没有把 movement 槽位用于 `asset_ref` 时成立；内容进入 `movement.content` 渲染，Capability 的资源数组只能由其他有媒体对象的槽位组成。若冻结版本仍要求 movement 媒体引用，必须拒绝而非隐式改变旧版本；新发布的 video Template 应仅将 `first_frame` 映射为资源引用。媒体型 movement 仍按下段对象校验处理。纯文本输入不得生成伪 Object Path 或伪签名 URL。

输入中的短期签名 URL 只用于 Resume 当下验证对象属于本企业 Storage 命名空间且可读；Job、Outbox 与 Conductor 启动输入必须冻结不含凭证的 Object Path。Personal/Managed Worker 领取历史 Capability Task 时由 `media_workflow` 使用内部凭证向 `media_materials` 重新解析受控 Object Path，检查归属与剩余有效期后才交付短期 URL。内部解析接口不能绕过用户提交时的资源可达性决策；解析失败不交付任务、不回退旧 URL。

Conductor Backend Compiler 必须为可恢复版本提供受控恢复分支：按 Job 中服务端校验的成功前缀，把已登记 Asset 还原为与普通 Finalize 相同的 `output_resource` 结构；不调已完成节点的 Render、Capability、HUMAN 或 Finalize，不把旧 Conductor ID 当作可恢复实例。之后从首个未完成节点执行，沿现有 `taskToDomain`、Worker Gateway、审批与结果登记链路。恢复分支读取业务数据库且只接受当前 Job/StepRun 的受控 Asset；新 Conductor correlation ID 由 `job_id + resume_generation` 派生，Outbox 结果不确定时按该 ID 查询。

## 核对与安全

- 导入前先校验源归属和目标账户映射；对象复制后核对 hash、Content Type 和可读性，拒绝仅保存旧局域网 URL。
- 全量迁移必须输出源总数、纳入/排除数、来源 ID 对照、Object Path 验证和逐组状态；失败组可重试，不能静默跳过。
- E2E 先选一个完整上游但无下游 Video 的 FirstFrame：经 API 导入后 UI 显示前三个已完成节点和视频待运行；补 Movement 后 Resume，使用 `media_worker` fixture 只领取 video Capability，完成并显示可访问结果。重复导入/Resume 不得新增 Job 或重复视频；断链样本不得进入目标。
- Mock 视频结果须先摄取到目标对象存储，再由 fixture Worker 返回该受控对象的短期 URL；`media_materials` 对原生 Generated Asset 复核企业/Scope 命名空间、存在性与媒体类型，只保存 Object Path。不能把旧系统 URL 或过期签名 URL 当作最终资产事实。
- 公开 Generated Asset 读取须先按企业成员关系及 private/team/public 可见性过滤，然后对持有 Object Path 的产物按请求签发临时 URL；内部来源查询和迁移写入继续使用独立服务凭证。列表计数不得包含当前用户不可见的记录。

## 全量能力覆盖门禁

| 旧事实 | 目标边界 / 聚合或读模型 / Workflow | 状态 |
|:---|:---|:---|
| URL 为空、内容非空的 Movement | media_materials asset/Material；media_workflow execution/Job Input 与 workflow_execution/Resume | covered（纯文本，不伪造对象） |
| 未完成 Model/Style/FirstFrame 链 | media_materials Imported Generated Asset；media_workflow Execution/Job + Resume | covered（单组 API/CLI，批量对账待实现） |
| 已完成 Video 与独立 VideoTask | media_materials Imported Generated Asset；media_workflow 历史 Execution/StepRun | missing（终态 Job API 已实现，视频资产摄取、CLI 和真实验收未完成） |
| VideoPush 多视频成员与原发布事实 | media_workflow historical_publication/HistoricalVideoPush；historical_video_push_import | missing（聚合、接口、媒体摄取和真实对账待实现；剪辑/重新发布流程不在本轮范围） |
| 断链排除 | 迁移 CLI 排除清单；不创建目标聚合 | covered |

存在 `missing`，不能宣称领域迁移能力或全量写入门禁完成。

## 单样本验收记录（2026-10-02）

测试企业 `8afeb26b9df643952f4f816576eb2504` 已为联调启用 `workspace` 和 `workflow` 功能。通过统一网关和测试 User API Token，恢复已导入的 `job_9f225df6-e1a6-4627-9a40-24487720ccd8`：恢复前 3 个历史图片 StepRun 均为 succeeded，video 为 blocked；只启动允许 `media.video.generate@1` 的客户端 fixture Worker，补入已摄取的 movement 对象后，Job 成为 succeeded。历史图片的 Asset ID 保持不变；video 新产物为 `gen_0a286716-6d67-4f75-b0d7-54109a63ce9e`，其持久化 Object Path 与预摄取视频一致，公开读取签发 URL 且 Range GET 可访问。测试实现见 `media-cli/e2e/workflow_history_resume_e2e_test.go`。完成后相同幂等键重放返回 202 且 Conductor 实例不变，不同键返回 409；隔离 PostgreSQL 的 Outbox 并发与完成后重放测试也通过。该 fixture Worker 已在验收后停止，后端服务保持运行；本记录不代表其余来源组已经导入或 UI 已验收。
