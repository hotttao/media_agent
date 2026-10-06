# media_worker

客户端 Capability Worker。它只访问统一网关，不配置或访问 Conductor。

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
