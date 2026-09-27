# media_worker

客户端 Capability Worker。它只访问统一网关，不配置或访问 Conductor。

联调示例：

```bash
MEDIA_GATEWAY_URL=http://192.168.2.41:18080 \
MEDIA_WORKER_TOKEN=tos_xxx \
MEDIA_WORKER_PROFILE=media_ai_fixture \
MEDIA_WORKER_CAPABILITIES=media.image.model@1,media.video.generate@1 \
go run ./cmd/media_worker
```

`MEDIA_WORKER_TOKEN` 必须是绑定当前 Organization、尚未过期，并包含所需
`workflow.worker.poll`、`workflow.worker.heartbeat`、`workflow.worker.complete`
和 `workflow.worker.fail` scopes 的 Service Token。`MEDIA_WORKER_CAPABILITIES`
为空时，fixture profile 注册其全部内置 Capability Version。

Worker 启动后先注册能力，再按 Capability 并发轮询。任务只使用不透明
`dispatch_id` 回报心跳、完成或失败；结果回报发生暂时性网络错误时，会使用同一个
幂等键重试，不会重新执行 Provider。
