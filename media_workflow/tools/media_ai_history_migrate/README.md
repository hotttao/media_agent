# media_ai 历史数据迁移：源清单与单组图片摄取

工具只读旧 `media_ai` MySQL，导出 WorkflowTemplate、ModelImage、StyleImage、FirstFrame、VideoTask、Video 和 VideoPush，并按实际引用恢复每个 Video 的上游谱系、匹配的模板与剪辑分组。默认模式不修改源库或目标库。`--import-group` 只把一个完整、未生成下游视频的图片组，经目标网关上传并通过迁移专用内部 API 登记为历史 Generated Asset；加 `--import-job` 才会导入 `awaiting_resume` Job，且不会自动启动 Conductor。

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

`--audit-readiness` 只读旧库、目标网关的素材列表和旧媒体服务；按全部候选分组核对 Product/Virtual IP/pose/scene/movement 来源 ID，并用 Range GET 实际读取候选图片、视频和输入素材的首字节。报告只输出来源 ID 与失败原因，不输出 API Token 或媒体 URL。`ready_groups` 表示来源媒体齐全的分组，不等同于已经导入或能够直接执行；`media_blocked_groups` 列出缺少来源媒体的分组。缺失的源对象不会被工具猜测或伪造。

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
