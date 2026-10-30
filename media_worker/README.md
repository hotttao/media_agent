# media_worker

## 执行日志关联

网关领取响应中的只读 `_traceparent` 元数据由客户端提取为 Task 关联字段，并从Provider输入中移除。心跳、上传准备、完成/失败回写沿用标准W3C header；直接Storage PUT既不带API Token也不带追踪header。完成/失败/未确认日志记录trace_id和不透明Dispatch，不打印提示词、候选或上游错误正文。

私有结果日志只保留关联字段和待交付结果，仍不复制任务输入；重启恢复使用原关联。关联不是身份、幂等键或执行资格，坏/缺字段不阻断旧任务。客户端不引入exporter，服务端使用既有本地非采样关联；源码已完成，跨服务运行日志证据见追踪plan，不以单测代替部署验收。

## 失败回写契约

成功回写候选结果；失败请求可携带`failure_kind`：`transient`由Conductor的Task Definition决定自动重试，`permanent`停止该Task的自动重试。旧请求省略分类仍为永久失败。客户端不会自动重跑生成函数；HTTP发送重试只是使用同键交付相同结果。

executor可通过错误的`Permanent() bool`方法声明分类，包装错误通过`errors.As`识别；明确返回false时发送transient，未分类fixture错误保持permanent。回写经网关由media_workflow直接代理Conductor，200/submitted表示精确Task结果已确认，不表示整个Workflow完成。服务端不再写结果Inbox或启动后台发送器。render_template/Finalize仍由服务端内部Worker执行。

未确认结果先保存到`--state-dir`/`MEDIA_WORKER_STATE_DIR`（默认当前目录`.worker-state`），目录0700、文件0600，按网关/实例/凭据摘要隔离，不保存Token或复制提示词输入。客户端重启先恢复交付，不重新调用executor；发送失败保留相同结果/幂等键，确认后删除自己的待交付文件，409/410保留`.rejected`文件供检查。相同命名空间仅允许一个客户端进程持有文件锁。不要删除/更换该目录来规避恢复；轮换凭据后旧命名空间需使用原授权身份核对。生成副作用完成但文件落盘前的窗口仍要求Provider幂等，不声称端到端exactly-once。

客户端 Capability Worker。它只访问统一网关，不配置或访问 Conductor。

## 新 Provider 契约联调

新样本使用 `MEDIA_WORKER_PROFILE=provider_fixture`，复用原注册、轮询、心跳和结果回写，不执行 `media_render_template_v1` 或 Finalize。支持 `jimeng.i2i.first_frame_prompt@1`、`jimeng.i2i.multi_image_prompt@1`、`doubao.i2v.first_frame_prompt@1`、`doubao.i2v.multi_image_prompt@1`。这是项目 mock 协议，不是已接入真实模型，也不证明豆包原生 API 的多图模式。

先创建单个测试 Job，再用其精确 ID 创建私有配置 `media_worker/.fixture.local.json`（0600，已忽略），通过 `MEDIA_WORKER_FIXTURE_CONFIG` 指定路径。配置示意：

```json
{
  "tasks": [
    {
      "job_id": "<本次测试JobID>",
      "step_key": "model_image",
      "capability_key": "jimeng.i2i.multi_image_prompt",
      "capability_version": 1,
      "result_urls": ["https://storage.example/<本次企业已上传模特图对象>"]
    },
    {
      "job_id": "<同一测试JobID>",
      "step_key": "style_image",
      "capability_key": "jimeng.i2i.multi_image_prompt",
      "capability_version": 1,
      "result_urls": ["https://storage.example/<本次企业已上传定妆图对象>"]
    }
  ]
}
```

同一生图能力按精确 Job/Step 返回不同结果；输入 prompt 已在服务端 Render，images 是按映射顺序交付的数组。每条绑定须有1–8个不同存储对象的结果 URL，多候选仍走用户审批。未知 Job/Step、版本不符、内部任务、缺失prompt、首帧传入多张图片均拒绝，不回退旧固定样本。启动时仅注册配置中使用的能力；`MEDIA_WORKER_CAPABILITIES` 可进一步缩小，但不能扩大。不能混用旧的 `MEDIA_WORKER_FIXTURE_*_RESULT_URL(S)` 覆盖变量。

统一网关/Worker Token配置沿用下文；从仓库根目录运行：

```bash
export MEDIA_WORKER_PROFILE=provider_fixture
export MEDIA_WORKER_FIXTURE_CONFIG="$PWD/media_worker/.fixture.local.json"
go -C media_worker run ./cmd/media_worker
```

配置文件需在轮询前写好，运行期间不修改。仍须先审计该用户的整个待执行队列：当前poll按Capability而非Job筛选，Job/Step绑定不是服务器队列隔离机制。不要让本次fixture领取其他用户任务；匹配不到的任务会返回测试失败。结果签名URL可能过期，实际联调前须通过Materials重新解析；不要输出或提交配置内容。失败注入沿用下文fixture参数，计数仍仅保存在当前进程中。

## 旧契约兼容联调

### 本地文件结果（实现待公开网关整体验收）

Provider 配置可使用 `"result_files": ["/private/results/frame.png"]` 替代 `result_urls`，两者互斥。客户端嗅探 PNG/JPEG/WebP/GIF/MP4/WebM，限制图片16MiB、视频256MiB，计算SHA256/MD5，通过绑定Dispatch/attempt/候选位置的网关上传接口申请短期PUT。此接口要求已有的 `workflow.worker.upload` scope。

文件直接PUT对象存储，不转发Worker Token、不自动跟随重定向。成功、412或响应丢失后重新申请，由服务端HEAD核对元数据再确认；结果只回写 `object_path`，查询时服务端在响应副本解析短期访问URL。上传不确认时不会回写成功。当前单测/race覆盖此路径，尚不代表部署后的完整流程验收。

上传发生在结果日志落盘之前；进程在此窗口退出仍依赖Provider侧幂等与固定结果文件恢复，不承诺生成exactly-once。新文件内容生成新对象键，已取消任务的旧签名可能产生未引用对象，不自动删除用户数据。

联调示例：

测试Worker凭据已通过公开API签发，保存在本目录被Git忽略的`.env`（0600）。它与User接口测试Token不同，绑定稳定Worker身份和企业；不要复制User Token冒充Worker。需要新增身份/凭据时，先用User API Token执行CLI：

```bash
media-cli media_auth worker-identity create --name desktop --organization-id <organization_id>
media-cli media_auth worker-identity create-token --id <worker_identity_id> \
  --name desktop --expires-at <RFC3339> --env-out <new-private-env-path>
```

CLI的Secret文件使用通用`MEDIA_API_TOKEN`变量；从仓库根启动Worker时映射到Worker配置，Secret不会进入命令行参数：

```bash
set -a
. media_worker/.env
set +a
export MEDIA_WORKER_API_TOKEN="$MEDIA_API_TOKEN"
export MEDIA_GATEWAY_URL=http://192.168.2.41:18080
export MEDIA_WORKER_PROFILE=media_ai_fixture
export MEDIA_WORKER_FIXTURE_ASSETS_URL=http://192.168.2.38
export MEDIA_WORKER_FIXTURE_VIDEO_RESULT_URL='<本企业目标对象存储中已上传视频的短期 URL>'
export MEDIA_WORKER_CAPABILITIES=media.video.generate@1
go -C media_worker run ./cmd/media_worker
```

首次单流程验收前应检查现有待执行任务；fixture会消费当前User的Personal Capability任务，不能在未核对队列时宣称仅执行一个样本。`worker-identity disable/delete`分别停用/软删除身份并撤销关联Token；当前版本不重新激活身份。

完整四节点对象存储联调需在启动前提供四个**已摄取到目标企业存储**的旧结果 URL：

- `MEDIA_WORKER_FIXTURE_MODEL_RESULT_URL`：模特图。
- `MEDIA_WORKER_FIXTURE_STYLE_RESULT_URL`：风格图。
- `MEDIA_WORKER_FIXTURE_FIRST_FRAME_RESULT_URL`：首帧图。
- `MEDIA_WORKER_FIXTURE_VIDEO_RESULT_URL`：视频。

URL 通过 Materials API 在测试开始前重新解析，短期签名可能过期，不提交或打印 URL。放在私有环境中，而非进程命令行参数；Worker 不需要 User 上传凭据。覆盖结果仍带 `mock=true` 和旧样本 provenance，不能当作真实生成结果。未配置某个结果时仍使用旧资源服务的对应固定样本，只适合诊断旧链路，不代表目标对象存储验收。配置完成后再启动 Worker，不在轮询期间修改配置。

多候选联调可设置 `MEDIA_WORKER_FIXTURE_MODEL_RESULT_URLS` 为 JSON 字符串数组（2–8 个不同的受控图片对象 URL），同时取消单值 `MEDIA_WORKER_FIXTURE_MODEL_RESULT_URL`。只更换签名的同一对象不能冒充多个候选；列表按顺序返回、首项 primary，其余非 primary，仍明确标记 mock。该变量只放私有环境中，禁止打印或作为命令行参数传入；Worker 不自动选择或审批候选。

失败重试联调可显式设置 `MEDIA_WORKER_FIXTURE_FAIL_STEP=model_image` 与 `MEDIA_WORKER_FIXTURE_FAIL_CALL=2`。仅 fixture profile 生效：每个 Job 的该节点第 2 个不同 Dispatch 返回固定测试错误，同一 Dispatch 重交付仍返回相同结果；后续新 Dispatch 正常返回已配置媒体。用于验证“首轮→重新生成→第二轮失败→用户重试”，不会执行内部 Renderer 或自行重试业务节点。计数只在当前客户端进程内，重启会重置；不是生产故障策略。启动前仍须核对队列，结束后停止测试 Worker。

```bash
MEDIA_GATEWAY_URL=http://192.168.2.41:18080 \
MEDIA_WORKER_API_TOKEN=talos_xxx \
MEDIA_WORKER_PROFILE=media_ai_fixture \
MEDIA_WORKER_CAPABILITIES=media.image.model@1,media.video.generate@1 \
go run ./cmd/media_worker
```

`MEDIA_WORKER_API_TOKEN` 必须绑定 `Worker:<worker_id>` Actor 和当前 Organization、尚未过期，并包含所需
`workflow.worker.poll`、`workflow.worker.heartbeat`、`workflow.worker.complete`
和 `workflow.worker.fail` scopes 的 API Token。`MEDIA_WORKER_CAPABILITIES`
为空时，fixture profile 注册其全部内置 Capability Version。

Worker 启动后先注册能力，再按 Capability 并发轮询。任务只使用不透明
`dispatch_id` 回报心跳、完成或失败；结果回报发生暂时性网络错误时，会使用同一个
幂等键重试，不会重新执行 Provider。

`MEDIA_WORKER_FIXTURE_ASSETS_URL`（或 `--fixture-assets-url`）仅指定 Mock 样本的旧资源服务根地址，默认与素材迁移工具一致为 `http://192.168.2.38`。历史单视频联调先经 Library Object Upload 把一份真实旧视频摄取到目标对象存储，再把该对象的短期 URL 交给 `MEDIA_WORKER_FIXTURE_VIDEO_RESULT_URL`；Worker 只返回已确定的测试结果，不持有 User 上传凭证。`media_materials` 在登记原生 Generated Asset 时复核对象位于本企业/Scope 且媒体类型正确，只持久化 Object Path。生产 profile 复用网关授权上传、内容摘要与确定 object_path，不使用 User 上传凭据。

## 分层与迁移状态

CLI 位于 `cmd/media_worker`，执行生命周期位于 `biz/workflow/execute`，能力选择和失败分类位于 `biz/policy/execution`。`biz/shared/worker_runtime` 定义公共技术执行、上传与结果交付端口；HTTP 和磁盘 DTO 分别位于 `biz/client/worker_gateway`、`biz/dal/repo/results`，保留 `.worker-state` 命名空间、权限与旧日志格式。生成检查点由独立 `biz/dal/repo/doubao_checkpoints` 实现，记录私有生成意图、引用、manifest 和已确认对象路径。

`biz/domain/doubao` 定义可替换 Generator，`biz/client/doubao/studio` 实现专用实例的本机 Worker 协议。`biz/workflow/doubao_generate` 协调原 Attempt 资格确认、输入下载、稳定 Key 查询、提交、确切产物下载、manifest 和授权上传。真实适配支持 `doubao.i2v.first_frame_prompt@1`（恰好一张图）与 `doubao.i2v.multi_image_prompt@1`（至少一张有序参考图），输出 VIDEO。多图需要同步部署配套 Studio 受控接口扩展；真实验收状态见扩展计划。

## Studio 生产 profile

先安装并构建用户 fork `hotttao/doubao-studio`，使用独立用户目录扫码。将 `conf/local.example.yaml` 复制到私密 `conf/local.yaml` 后填写专用目录、账号 ID、精确 Electron 命令和生成参数；配置走共享 loader 与 protobuf schema。网关 URL、Worker Token 和实例 ID 仍由原 CLI/env 提供，Token 不写入 yaml 或运行日志。

```bash
export MEDIA_WORKER_PROFILE=doubao_studio
export MEDIA_WORKER_STUDIO_USER_DATA=/private/dedicated-studio
export MEDIA_WORKER_STUDIO_ACCOUNT_ID=<account-id>
export MEDIA_WORKER_STUDIO_DIRECTORY=/path/to/doubao-studio
export MEDIA_WORKER_STUDIO_ELECTRON=/path/to/doubao-studio/node_modules/electron/dist/electron
# 在确认独立 Worker 身份与待执行队列后启动；需要已设置原网关/Token/Worker ID。
go run ./cmd/media_worker -config conf/local.yaml
```

启动时独占结果命名空间与 studio 用户目录；只声明一个并发槽，领取前确认 studio 就绪且没有待确认/生成中的旧任务。重启先恢复已确定结果，再恢复未关闭检查点，恢复资格由原 Dispatch/Attempt 的 Heartbeat 确认。拒绝资格停止副作用；网络不确定与需人工确认留在同次尝试，不回写失败触发重新生成。签名输入 URL 不持久化，不可清空状态目录绕过恢复。

真实生成、公开网关上传与审批是否验收通过，以 migration plan/review 的实际记录为准；代码检查不替代部署验收。

[迁移 spec](../docs/spec/media-worker-doubao.md) · [迁移 plan](../docs/plan/media-worker-doubao.md) · [领域与执行流程](../docs/ddd/media_worker/CONTEXT.md)。

验证：在本目录运行 `go build ./...`、`go test ./...`、`go test -race ./...`、`go vet ./...`、`go run ../harness/tools/archcheck` 和 `go run ../harness/tools/deadcheck -filter=media_agent/media_worker`。跨进程组合测试在 `e2e/`。


2026-10-26 首帧生产验收已完成：真实视频经公开网关上传、审批、登记，Job SUCCEEDED；原 Worker 重启后复用同一 Studio 生成。用户 fork 固定为 [840117a](https://github.com/hotttao/doubao-studio/commit/840117a)，分支 `codex/media-worker-studio`。证据与范围见 [完整 review](../docs/review/media_worker/workflow/0002-generate/generate.md)。

后端 `media_materials` 与 `internal_worker` 的 `MEDIA_STORAGE_PUBLIC_ENDPOINT` 必须一致。通用生成登记从可信 Job 传递企业归属，Product/Virtual IP 可选；登记失败使用不带 input_override 的原生 retry 继续原实例，无需重新启动能力 Worker 或重生成。

## GPT 成图 profile（独立流程）

GPT 实现位于 `biz/domain/gpt`、`biz/workflow/gpt_generate`、`biz/client/gpt/image_use` 与 `biz/dal/repo/gpt_checkpoints`。豆包仍使用独立 `doubao_generate`；不通过通用生成流程或 Recipe 兼容。公共 Worker 技术契约位于 `biz/shared/worker_runtime`，不是业务 Domain。

首版能力：

- `gpt.t2i.text_prompt@1`：已渲染 prompt，输出一张图片。
- `gpt.i2i.multi_image_prompt@1`：prompt + 1～4 张有序参考图，输出一张图片；不会静默丢弃多余参考图。

固定 `image-use --backend web --profile relay`，连接当前已登录 Chrome。**单账号串行**，没有 Studio 式账号池。不会使用 auto/Codex/Gemini，也不会读取或转发 Codex 凭证。图片尺寸在网页端是提示，不保证像素、质量或透明背景。上游版本固定，不能直接执行 upgrade 后沿用旧 bridge。

```bash
# 安装固定版本 image-use；校验上游源码 SHA-256，不安装 Chrome 插件。
python3 tools/install_image_use.py
# chrome-use CLI 与 Chrome 扩展须先按其官方安装说明完成连接。
python3 tools/check_gpt_browser.py --image-use /absolute/path/to/pinned/image-use --open
```

浏览器准备遵循 [chrome-use 官方文档](https://github.com/leeguooooo/chrome-use)：安装 CLI、执行 `chrome-use extension install` 注册 native host，并在当前 Chrome 安装/启用扩展后登录 ChatGPT。不要复制 Cookie，不要用新建的无登录 headless 浏览器替代 relay；首次插件授权需要用户在 Chrome 完成。

运行 Worker 的环境必须能在 `PATH` 找到 `chrome-use`（安装在 `~/.local/bin` 时需将该目录加入 `PATH`）。准备命令的 `--open` 会打开专用 ChatGPT 会话；正常空队列轮询只读探测，不刷新页面，有草稿或仍在生成时不会领取新任务。

将 `conf/gpt.example.yaml` 复制为私密 `conf/local.yaml`，配置 Python/bridge 绝对路径、上游 CLI、共享浏览器锁目录、网页模型标签与可选现有 Project URL。bridge 文件位于本仓库 `media_worker/tools/image_use_bridge.py`。模型默认候选 `Instant,Auto` 沿用固定上游的支持值；不是官方图片模型 ID。网关 URL、Worker Token、稳定 Worker ID 与状态目录继续沿用原配置。

```bash
export MEDIA_WORKER_PROFILE=gpt_image_use
export MEDIA_WORKER_GPT_BRIDGE=/absolute/path/to/media_worker/tools/image_use_bridge.py
export MEDIA_WORKER_IMAGE_USE_CLI=/absolute/path/to/pinned/image-use
export MEDIA_WORKER_GPT_LOCK_DIRECTORY=/private/shared-gpt-browser-lock
# 核对独立 Worker 身份和待执行队列后运行；需要设置原网关/Token/Worker ID。
go run ./cmd/media_worker -config conf/local.yaml
```

服务端发布能力需要 `media_workflow` migration 000028 及支持有限 `max_items` 的契约校验代码；发布后由既有 Conductor Task Definition 注册流程处理。不要只插入数据库记录而仍运行旧服务二进制。该迁移沿用既有不可变 Provider 契约规则：存在已发布版本时拒绝 down；需要停用时通过后续迁移归档，不删除已经被引用的版本。

GPT Workflow 检查点位于凭证命名空间的 `gpt-checkpoints`，bridge 技术台账位于 `image-use`。发送意图先于浏览器动作，之后只恢复原 conversation/user turn；多出一轮用户消息、会话变更、无绑定或不确定发送会保留 needs_human。原会话与人工状态需要检查，不能清空状态目录或重新提交相同请求绕过。图片 manifest 通过大小、媒体类型、SHA-256 与私有文件校验后才上传；公开 Gateway 仍决定执行资格和对象路径。

[GPT 设计](../docs/spec/media-worker-gpt-image-use.md) · [落地状态](../docs/plan/media-worker-gpt-image-use.md) · [公共运行契约](../docs/ddd/media_worker/runtime.md)。

### Materials 查询 MCP（2026-10-30）

同一个公网 `/mcp` 现在可按模块授权聚合两个来源：`media-workflow` 的 13 个批量工具，以及 `media-materials` 的 6 个只读批量工具：`virtual_ips_list/get`、`materials_list/get`、`generated_assets_list/get`。它们分别调用原 Materials 列表/详情 API，保持租户、对象 ACL 和分页语义。素材来源不依赖 Workflow 模块使用权限，且不会获得 Workflow 目标凭据。原 CLI MCP 配置无需改 URL；已打开的会话需要刷新/重连工具目录。

多图执行请同步更新 Studio 配套代码。Worker 未设置能力 allowlist 时注册两个 Doubao 能力；若设置了 MEDIA_WORKER_CAPABILITIES，需显式包含 doubao.i2v.multi_image_prompt@1。不能只更新 MCP 白名单而继续运行旧的单图 Studio 接口。
