# media_workflow

## Worker 执行边界

Personal Worker 是客户端：API Token → 统一网关 → Worker Gateway → Conductor。
Managed Worker 是本服务的可信内部运行时：只轮询配置允许的 Managed Pool，不申请 API Token、不调用 Talos、不经过公开 Worker API。
`media_render_template_v1`、finalize 等内部任务仍由 `cmd/internal_worker` 执行；不会下发给 Personal Worker。

## Managed Worker 配置与启动

入口是 `cmd/managed_worker`。配置 schema 在 `idl/config/config.proto` 的 `managed_worker` 中，示例见 [managed-worker.example.yaml](conf/managed-worker.example.yaml)。
默认关闭；没有真实服务端 Capability 实现时不要启动示例配置。启用前需满足：

- Capability 已发布，`execution_target=managed_worker`，版本和 `managed_pool_key` 与配置绑定一致。
- 每项 binding 指定一个启用的 `client.<executor_client>`；一个 task definition/domain 只能绑定一个消费者配置。
- 执行器提供内部 `POST /execute`，接受 `{dispatch_id, attempt, requested_by, capability_key, capability_version, input}`。
- 执行器依据 `Idempotency-Key` 去重，返回 `{candidates:[{url 或 content}]}`；失败返回非 200。
- 该执行器仅部署在可信内部网络。此协议不携带用户凭证，不能直接对外开放；外部模型供应商凭据由执行器自己维护。
- Workflow 需重新编译发布，以冻结 `capability_key` 和 `capability_version` 路由信息。不会就地修改已有发布版本。

```bash
CONFIG_FILE=/absolute/path/managed-worker.yaml CONDUCTOR_URL=http://127.0.0.1:8080 \
  go run ./cmd/managed_worker

# Compose 仅显式 profile 启动；独立配置文件不提交 Secret。
MANAGED_WORKER_CONFIG=/absolute/path/managed-worker.yaml \
  docker compose --profile managed-worker -f media_workflow/docker-compose.yml up -d --build managed_worker
```

Job 的 `submitted_by` 是请求用户，Dispatch 的 `actor_id=Worker:<配置 worker_id>`、`worker_instance_id` 是执行实例，`auth_source=internal_runtime`，没有 `credential_id`。
当前 Job 模型未独立保存 Organization；Managed Runtime 仅在冻结任务输入包含 `organization_id` 时保存该审计字段，不把它当成认证授权依据。
租约心跳、结果 Inbox、重复/冲突结果检查及异步 Conductor 回写复用 Personal 的实现；执行器故障不会绕过 Inbox。

## 测试

```bash
go test ./...
go test -race ./biz/workflow/worker_gateway ./biz/shared/client ./biz/workflow/conductor

# 独立 PostgreSQL（testcontainers）：并发幂等、相反终态拒绝、Conductor 回写失败重试。
go test -tags=e2e ./e2e -run TestDispatchInbox -count=1 -v

# 真实 Conductor + 隔离测试数据库，只有模型执行器 mock。
MEDIA_E2E_CONDUCTOR_URL=http://127.0.0.1:8080 \
  go test -tags=e2e ./e2e -run TestManagedWorkerConductorRoundTrip -count=1 -v
```

真实引擎测试创建唯一命名的任务和 Workflow 定义并在结束后清理，不操作用户已有 Job/Capability/Token。
HTTP/CLI E2E 的统一网关入口和 API Token 使用方式见 [测试规范](../harness/contributing/e2e.md)。
