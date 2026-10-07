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

`MEDIA_WORKER_FIXTURE_ASSETS_URL`（或 `--fixture-assets-url`）仅指定 Mock 样本的旧资源服务根地址，默认与素材迁移工具一致为 `http://192.168.2.38`。历史单视频联调先经 Library Object Upload 把一份真实旧视频摄取到目标对象存储，再把该对象的短期 URL 交给 `MEDIA_WORKER_FIXTURE_VIDEO_RESULT_URL`；Worker 只返回已确定的测试结果，不持有 User 上传凭证。`media_materials` 在登记原生 Generated Asset 时复核对象位于本企业/Scope 且媒体类型正确，只持久化 Object Path。生产 Worker 的对象上传协议仍需单独实现。
