# media_ai 历史数据迁移：源清单、图片、视频与历史发布记录

工具只读旧 `media_ai` MySQL，导出 WorkflowTemplate、ModelImage、StyleImage、FirstFrame、VideoTask、Video 和 VideoPush，并按实际引用恢复每个 Video 的上游谱系。默认模式不修改源库或目标库。`--import-group` 导入一个未生成下游视频的图片组；加 `--import-job` 才导入 `awaiting_resume` Job。`--import-video <源 Video ID>` 导入一条完整视频与三张上游图片，并通过迁移专用 API 建立 `succeeded` Job。`--import-push <源 VideoPush ID>` 先导入有序成员视频 Job，再摄取该发布记录非空的成片、缩略图及手工剪辑媒体，最终登记独立的历史发布记录。这些模式都不会启动 Conductor；多视频剪辑与重新发布流程待单独设计。

```bash
export MEDIA_AI_DATABASE_URL='mysql://user:password@host:3306/media_ai'
go run ./tools/media_ai_history_migrate                  # 全量统计，不写文件
go run ./tools/media_ai_history_migrate --only-task ID   # 单条检查
go run ./tools/media_ai_history_migrate --out /secure/media-ai-history.json
MEDIA_MATERIALS_BEARER_TOKEN='<目标账户 API Token>' \
  go run ./tools/media_ai_history_migrate --audit-readiness \
  --organization-id '<目标 Organization ID>'
```

从 `media_workflow` 目录运行。`--out` 只允许新文件，权限为 0600；内容包含旧用户 ID、任务参数和结果地址，须按敏感数据保管。再次运行请给新文件名，避免覆盖上次审计清单。

`--audit-readiness` 只读旧库、目标网关的素材列表和旧媒体服务；按全部候选分组核对 Product/Virtual IP/pose/scene/movement 来源 ID，并用 Range GET 实际读取候选图片、视频、输入素材及完整 VideoPush 的非空媒体字段首字节。纯文本 movement 要求来源和目标内容一致、双方都没有媒体 Object Path，不发起无意义的 URL 探测。报告只输出来源 ID 与失败原因，不输出 API Token 或媒体 URL。`ready_groups` 表示来源媒体齐全的分组，不等同于已经导入或能够直接执行；`media_blocked_groups` 和 `media_blocked_pushes` 分别列出缺少来源媒体的分组及历史发布记录。缺失的源对象不会被工具猜测或伪造。

已核对当前源库：1 个 `workflows`（`Manual Upload`，无 DSL config）、1 个 `workflow_templates`、153 个 `video_tasks`（均为 `params.source=manual_upload`）、127 个 `videos`。`VideoTask` 的 `source` 标识单次视频任务的入口，不等同于业务流程类别；它可以处于完整链路中。旧七阶段流程由 `scripts/workflow-cli` 沿资源关系恢复与推进，而非读取 `workflows.config`。当前 [media_ai_workflow_migrate](../media_ai_workflow_migrate/main.go) 只负责四节点定义和联调样本。

单组图片摄取示例（目标 Product / Virtual IP 必须事先映射并处于同一 Organization、`ai_clothing` Scope）：

```bash
export MEDIA_AI_DATABASE_URL='mysql://user:password@host:3306/media_ai'
export MEDIA_MATERIALS_BEARER_TOKEN='<目标账户 API Token>'
export MEDIA_MATERIALS_MIGRATION_TOKEN='<仅限本次迁移的内部密钥>'
go run ./tools/media_ai_history_migrate \
  --import-group 'frame:<源 FirstFrame ID>' \
  --organization-id '<目标 Organization ID>' --owner-user-id '<目标 User ID>' \
  --product-id '<已映射的目标 Product ID>' --virtual-ip-id '<已映射的目标 Virtual IP ID>' \
  --source-assets-url 'http://<旧媒体服务>' \
  --target-gateway 'http://<统一网关>' \
  --materials-internal-url 'http://127.0.0.1:8003'
```

单条已完成视频使用相同目标身份与映射配置，增加 `--import-video '<源 Video ID>'`、`--workflow-id`、`--workflow-version` 和 `--workspace-id`；无需 `--import-job`。工具先检查该 Video 属于完整候选链且未被断链清单排除，再按来源 ID/摘要复用三张上游图片、摄取视频，最后导入四个已完成 StepRun。历史发布记录使用相同参数但指定 `--import-push '<源 VideoPush ID>'`。媒体在迁移工具和目标上传服务均有界落盘；图片上限 16 MiB，视频上限 256 MiB。超过上限明确失败，不截断也不误记完成。

批量入口 `--import-all-report /secure/new-report.jsonl` 不接受单条 `--product-id` / `--virtual-ip-id`；它先从目标 Product 的迁移来源 ID 和目标 Virtual IP 的唯一昵称建立映射，任何歧义立即停止。随后导入未完成组、去重后的单条 Video，以及独立 VideoPush。新建的 0600 JSONL 报告逐条记录排除原因、导入结果、目标 Job/发布记录 ID、每个已上传媒体的 Object Path/SHA-256 或失败原因并同步落盘。失败后用**新的**报告路径重跑；目标 API 按来源事实幂等。来源成片 404 的发布记录报告失败并使最终命令返回非零，不得伪造完成。

2026-10-03 已完成真实全量批次：121 个未完成组、125 个 Video、52 个 VideoPush 成功；50 个断链记录排除，1 个来源成片 404 的 VideoPush 隔离。重跑会从目标公开 Job 列表取得已导入来源的原 Workflow 版本，保留旧冻结定义；原未完成 Job 若已续跑成功，幂等确认返回当前状态。

只读对账使用 `--verify-report /secure/report.jsonl`，同样提供目标 Organization、Owner、Workspace 与 User API Token；不需要迁移密钥。它核对源记录集合是否完整、目标 Job 与资产来源/归属/原时间、VideoPush 原字段和有序成员，并逐个读取目标媒体首字节及 S3 SHA-256 元数据。存在隔离项时保留验证结果并返回非零。实际全量对账核对了 298 个成功记录、50 个排除项、1 个隔离项及 538 个唯一对象。

内部 API 默认禁用，需让 `media_materials` 进程配置相同的 `MEDIA_MATERIALS_MIGRATION_TOKEN`；`--import-job` 还需让 `media_workflow` 配置 `MEDIA_WORKFLOW_MIGRATION_TOKEN`，并传入 `--workflow-id`、`--workflow-version` 和 `--workspace-id`。完成后移除密钥并重启。密钥和 API Token 只经环境变量传入，不写在命令行或清单中。工具会逐张校验来源路径、图片类型、SHA-256、上传返回摘要和目标 Product/Virtual IP 对应关系。重复执行使用稳定上传幂等键与来源 ID 唯一约束；同来源不同内容会报冲突。Job 导入还会向 `media_control` 校验目标 Workspace、所有者和企业归属。

后续全量迁移分两条通道：

1. 从 Product/Virtual IP、ModelImage、StyleImage、FirstFrame、Video、VideoPush 的关联重建每条旧流程实例，识别复用、并行动作分支、剪辑分组和孤儿记录；`VideoTask` 关联单个 Video，不是整条流程的根。
2. 可表达为新 Workflow DSL 的旧 CLI 定义及其输入，经 `media_workflow` 公开 API 创建、发布，之后按需重新执行；重跑产生**新** Job，不能冒充历史执行。
3. 历史产物与执行谱系需要专用、幂等的导入 API，保留来源 ID、原时间、归属、依赖和结果媒体；不能调用 `SubmitJob` 伪造历史，也不能直写目标数据库。导入前还要将源文件搬到目标对象存储并校验摘要。

目标现在允许独立登记 `legacy_import` Generated Asset，并可导入未完成 Job；不伪造原生历史执行或重新运行已完成图片。显式 Resume、完整 Worker 联调和全量迁移仍需按 Spec 门禁验证。`video_lineages` 保留源关系供审计；`lineage_groups` 只包含上游完整的迁移候选；`excluded` 按源记录列出跳过原因，`issues` 是逐引用诊断。断链不进入目标，源库不做物理删除。

2026-10-01 源库清点：42 个逐引用问题最终对应 50 条被排除的源记录（26 个无 Video 的 VideoTask、2 个断链 Video、22 个无效 VideoPush）；剩余 125 个 Video 组成 53 个完整 VideoPush 分组和 8 个未成片视频分组。另有 98 个 StyleImage 和 23 个 FirstFrame 没有下游，这 **121 条不是断链**；清单分别标为 `partial_style_image` / `partial_first_frame`，`execution_state=awaiting_resume`，并标出下一个 Step。若存在无下游 StyleImage 的 ModelImage，也标为 `partial_model_image`。这些分组在目标中应导入为未完成 Workflow Execution，以便用户补齐输入后继续执行；目前仅一组图片及其待继续 Job 已写入目标。`--only-task` 仅供抽样检查，其组别是 `sample_projection`，不能当作完整 VideoPush 导入。

已通过真实 API 验证的样本：`frame:00273837-74d3-4f60-9d30-5976f21c48c3` 的 ModelImage、StyleImage、FirstFrame 分别登记为 `gen_f4b9d262-d262-4a88-ac29-46628207be2d`、`gen_b5f75ae4-9cdf-43bd-92f4-0d86f42836c8`、`gen_4013aeee-f7a9-4bbb-83a2-2f8da63a4266`。导入密钥已在测试后从运行服务移除。

2026-10-02 已通过真实 API 在测试 Workspace `2e6c5e63-b343-47c4-9f4e-64eb3c7855ce` 导入 Job `job_9f225df6-e1a6-4627-9a40-24487720ccd8`；后来显式 Resume 经客户端 fixture Worker 只执行 video，Job 已成功，前三个历史图片 StepRun 未重跑。迁移密钥随后移除；此验证仅覆盖一条样本，并非全量迁移。

同一条样本的 fixture 视频已预先摄取到目标对象存储：`materials/8afeb26b9df643952f4f816576eb2504/ai_clothing/generated_asset_import/44b88deddf04f9c22b89e477fe88854e.mp4`，SHA-256 `d637fcd907e98ac7882ec54b78782bfa8c8699420f76c99fcccd0f25c3965dcc`，4,272,298 字节；目标解析 API 和 Range GET 已验证。执行前须重新签发短期 URL 并传给 `MEDIA_WORKER_FIXTURE_VIDEO_RESULT_URL`，不要把 URL 写入文档或持久配置。
