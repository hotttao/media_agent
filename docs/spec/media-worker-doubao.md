# media_worker 正式迁移规格

状态：P1 已确认；正在实现 P2–P5，真实部署验收单独记录。

## 目标与本批范围

正式代码位于 media_agent/media_worker，遵循 harness/contributing 的架构、配置和测试规范。doubao 只保存调研资料。先完成 spec/plan、整体结构和接口定义，再 review 并请用户确认；确认前不进入完整 studio 实现，不消费真实队列。

CLI 是协议入口：绑定参数、转换 Command、装配、调用用例、输出。豆包生成通过 DDD 领域接口表达；doubao-studio 是该接口的一种实现，未来可替换。

## 目录与依赖

```text
media_worker/
  cmd/media_worker/                 CLI / composition root
  biz/domain/execution/            执行事实、网关/执行器/仓储端口
  biz/domain/doubao/               生成请求、引用、状态、产物、Generator
  biz/policy/execution/            能力白名单、失败分类、恢复决策
  biz/service/                    按实际单领域用例增加
  biz/workflow/execute/            注册、恢复、领取、执行、心跳、交付
  biz/client/worker_gateway/       HTTP / 上传实现和 wire DTO
  biz/client/fixture/              现有 fixture 实现
  biz/client/doubao/studio/        后续 Generator 实现
  biz/dal/repo/results/            待交付结果仓储
  biz/shared/correlation/          无业务依赖的 trace/context 工具
  e2e/                            真实适配器组合、跨进程测试
```

不并列保留 internal 业务结构，不加转发 alias。domain 不依赖其他层、不访问文件或网络、不含 JSON/ORM tag。policy 无副作用；service/workflow 依赖领域端口，具体实现只在入口装配。client/dal 的协议及磁盘 DTO 与领域模型分离。execution 和 doubao 领域包不互相 import，跨域关系由 workflow 协调。

## 执行端口与兼容

- Executor：声明可执行 Capability，接收领域 Task 并返回 candidates/provenance。
- Gateway：Register、Poll、Heartbeat、Complete、FailWithKind。
- Uploader：按 Task/attempt/候选位置上传确定的本地文件，返回已确认 object_path。
- ResultRepository：Save、Pending、Finish，只存已确定的待交付结果。
- CheckpointRepository：独立保存生成意图、生成引用和产物事实，不承担调度。

动态能力的输入输出可保留 map；domain 不携带重复 input_json。HTTP 适配器解析输入并剥离 _traceparent，使用 wire DTO 映射领域 Task。409/410 映射为领域结果拒绝，workflow 不依赖 HTTPError。

保留现有 URL、Bearer、请求响应字段、submitted 回执判断、failure_kind、trace 脱敏和操作幂等键。Permanent() 明确返回 false 时为 transient，其余沿用 permanent。

保留旧 .worker-state 命名空间、路径哈希、文件权限和 snake_case 磁盘格式。独立 disk DTO 映射，必须用旧格式字面量测试验证直接恢复、同结果重复保存、原键交付；不能清空目录解决格式不兼容。

## 豆包端口

Generator 定义 Submit、Lookup、Observe、FetchArtifacts。Request 表达稳定 Key、媒体类别、已渲染 Prompt、有序本地参考图及显式模型/时长/比例。Reference 是实现不透明引用。Observation 表达已受理、生成中、需人工、成功、失败。Artifact 按确切 ID 关联下载文件。

端口不暴露 CSV、Electron、CDP 或 studio batch/task。自动回复参数确认是 studio 实现细节。Lookup 必须区分确切不存在和提交结果不确定；不能把超时当未提交再次生成。P1 仅定义端口，已确认进入实现；不提供返回成功的占位实现。

## 完整迁移边界

首个生产能力仅 doubao.i2v.first_frame_prompt@1，输入 prompt + images，恰好一张图，输出 VIDEO。多图视频需另行真实验证；豆包生图需新增对应能力，不冒充 jimeng.i2i.*。不把调研版 media.image.* / resources 当生产契约。

输入签名资源只在执行时下载，保持顺序，限制类型/大小/时间；不持久化签名 URL。先保存提交意图，再 Submit，返回引用后保存绑定。确切产物下载完成后保存 manifest，再复用网关授权上传，得到 object_path 后写待交付日志并回写。

检查点与结果日志分开；同次恢复不重复提交已知生成。重启不创造租约，恢复未完成生成前必须确认执行资格；现有公开协议不足时先补协议或转人工，不以再次 poll 推断旧任务资格。服务端明确的新 attempt 与同次恢复区分。

首版专用 studio 实例、全局串行，在领取前取得本地容量。禁止全局 pkill electron；进程必须有明确实例所有权和 readiness。禁止按下载目录最新 MP4 匹配结果。CDP 操作须绑定确切任务会话。

## 配置与 studio fork

结构迁移保留现有 CLI/env。新增共享配置遵循 config.proto → make pb-config → 共享 loader，不单独设计业务 YAML 加载。

上游：https://github.com/dorakoo/doubao-studio.git。本机已修改 main/ipc/tasks.ts、main/preload.ts、packages/contracts/src/dto/electron-api.ts、src/components/BrowserPanel.tsx、src/utils/doubaoBridge.ts。

用户要求在迁移完成后 fork 到其 GitHub，并把适配修改提交用户 fork。用户 fork 已提供为 hotttao/doubao-studio，记录源版本/补丁和最终 fork commit，不发布私密配置或浏览器数据。

## 验收

执行格式、编译、单测、race、vet、harness archcheck/deadcheck。workflow 用 fake 端口；客户端用 httptest；跨进程组合放 e2e。重点覆盖旧日志、恢复不调用 Executor、保存先于发送、拒绝结果保留、失败分类及 trace/Token 隔离。

真实生成和公开网关验收覆盖：真实输入→studio 视频→授权上传→候选→审批。代码、局部测试、部署验收分别报告；受环境阻断的门禁不得写作通过。

## 已确认的 P2–P4 接线

生产 `doubao_studio` profile 从共享 loader 读取 `media_worker` 配置段，经 `idl/config/config.proto → make pb-config` 生成。配置仅保存专用 studio 用户目录、账号 ID、精确 launch argv、工作目录、模型/秒数/比例与启动超时；网关 Bearer 继续沿用原 CLI/env。

本机控制协议扩展位于用户 fork `hotttao/doubao-studio`，基线 `7eeb7b67a518e47ee4296c36e5c5f17942a9cff0`。调研目录现为 `doubao-studio-me`，保留其原适配。API `/v1/worker/generations/{key}` 用确定 Key 创建/查询，`/advance` 驱动单次持久化动作，`/artifacts/{id}` 返回确切媒体字节；只在独立用户目录模式启用。主进程 PID、本机 token、profile/asset-root 规范目录 SHA-256 证明和 worker-v1 readiness 均须匹配。

输入限 HTTP(S)、30 秒、16 MiB、PNG/JPEG/WebP；产物限 256 MiB MP4，下载后保存文件 SHA-256 和大小。每次上传前复核 manifest。实例全局进程锁与容量为 1；Worker 不终止其他 Electron 进程。生成检查点与旧待交付日志共用凭证隔离命名空间下不同子目录。

恢复资格复用公开 Heartbeat：其服务端已按原 Actor/Credential/实例/Attempt 校验当前引擎活跃任务，不需要新增“再领取”协议。拒绝则停止，网络不确定则保留检查点。生产队列与审批验收须先确认网关地址、独立 Worker 身份和待执行队列。

## 通用产物登记的租户传递

Finalize 内部请求 `UpsertGeneratedAssetReq.organization_id = 16` 从已经验证的 Job 取得，由共享 client DTO 传入 Materials Command。通用首帧能力不隐式要求 Product/Virtual IP；有来源时校验来源 Scope/Organization 与目标一致，无显式企业的旧调用保留来源推导。Materials 继续复验受控存储 origin、企业/Scope 路径、对象大小/摘要与媒体类型，并按结果键原子登记。IDL model 与 OpenAPI 从协议生成，不增加公开入口。

运行时 Materials 与 internal_worker 必须配置同一 `MEDIA_STORAGE_PUBLIC_ENDPOINT`。登记失败通过公开 Job 原生 retry 继续 Finalize，省略 input_override，保留成功 Invoke 与审批；不重新轮询能力或提交 Studio。


## 2026-10-26 结构更正

用户明确移除 execution Domain：公共技术执行协议现位于 `biz/shared/worker_runtime`，Provider 检查点分别归各自 Domain。原 `workflow/generate` 正名为 `workflow/doubao_generate`，仓储正名为 `dal/repo/doubao_checkpoints`；物理检查点目录与格式保持兼容。GPT 成图独立实现，不能通过 Recipe 合入豆包流程。旧 P1 review 保留其当时审查记录，当前结构见 [运行契约](../ddd/media_worker/runtime.md)。
