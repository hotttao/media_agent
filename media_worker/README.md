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
go -C media_worker run ./cmd/media_worker
```

首次单流程验收前应检查现有待执行任务；fixture会消费当前User的Personal Capability任务，不能在未核对队列时宣称仅执行一个样本。`worker-identity disable/delete`分别停用/软删除身份并撤销关联Token；当前版本不重新激活身份。

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

`MEDIA_WORKER_FIXTURE_ASSETS_URL`（或 `--fixture-assets-url`）仅指定 Mock 样本的旧资源服务根地址，默认与素材迁移工具一致为 `http://192.168.2.38`。冻结结果文件路径与 provenance 不变；启动前需确认样本图片/视频可访问。它不替代生产 Worker 的对象上传机制，也不会修改已完成 Job 中保存的旧 URL。
