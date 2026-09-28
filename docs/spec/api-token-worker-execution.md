# API Token 与 Worker Execution Target 实现规格

## 1. 范围

本规格把 ADR 0018 落为 `media_auth`、`media_agent_gateway`、`media_workflow` 和客户端 `media_worker` 的实现契约，覆盖：

- Talos API Token 管理与验证；
- User、Agent、Personal Worker 的委托身份；
- Capability Version 的 `execution_target`；
- Personal/Managed Worker 的 Conductor Domain 隔离；
- Agent 创建时的 API Token 申请。

Platform Token、Web Session、OAuth Token 和 Agent Session Token 不纳入 API Token 聚合。Renderer/Finalizer 继续由 `media_workflow` internal worker 执行，不属于 Capability Execution Target。

## 2. 身份与凭证契约

### 2.1 API Token

`media_auth` 是 Token Manager，Talos 是 Secret 生命周期事实源。`media_auth` 只保存 `talos_key_id` 和授权元数据，不保存原始 Secret 或可用于本地旁路验证的 Hash。

API Token 元数据必须包含：

```text
id, talos_key_id
subject_user_id
actor_type = user | agent | personal_worker | service
actor_id
organization_id?
name, scopes[], status, expires_at
created_by, created_at, rotated_at?, revoked_at?
```

创建和轮换时 Secret 只返回一次。Talos 不可用时创建、验证、轮换和撤销均 fail closed。

### 2.2 Internal Identity JWT

直接用户：

```json
{"sub":"User:<user_id>"}
```

Agent 或 Personal Worker 代表用户：

```json
{
  "sub":"User:<user_id>",
  "act":{"sub":"Agent:<agent_id>"},
  "credential_id":"<api_token_id>",
  "organization_id":"<organization_id>",
  "scope":"space separated scopes",
  "auth_source":"api_token"
}
```

Personal Worker 将 `act.sub` 改为 `Worker:<worker_id>`。`client_id` 不表示 Actor，`worker_instance_id` 只表示本次进程实例。

### 2.3 验证边界

公开客户端只把 opaque API Token 发送给统一网关。Edge Auth 调用 `media_auth.VerifyAPIToken`，再签发 audience 精确、TTL 1–5 分钟的 Internal Identity JWT。业务服务不接收原始 API Token。

有效权限是 User 当前权限、Actor 状态、Token scope、Organization Membership/Entitlement 和资源授权的交集。

## 3. Talos Adapter

`media_auth/biz/shared/client/talos` 封装 Talos Admin API，业务层不能依赖 Talos SDK/HTTP DTO。初期需要四个接口：

```go
type Client interface {
    Issue(ctx context.Context, cmd IssueCommand) (IssuedKey, error)
    Verify(ctx context.Context, secret string) (VerifiedKey, error)
    Revoke(ctx context.Context, keyID string) error
    Get(ctx context.Context, keyID string) (KeyMetadata, error)
}
```

HTTP 路径和 DTO 以锁定的 Talos OpenAPI 版本为准。Adapter 必须设置超时、传播 `traceparent`、屏蔽 Secret 日志，并把无效/撤销/过期与上游不可用映射成稳定领域错误。

本地开发使用仓库维护的 Talos OSS 单节点容器与持久 SQLite volume；它属于认证基础设施，默认不随普通编码测试启动。

## 4. Capability Execution Target

### 4.1 持久模型

`execution_capabilities` 增加：

```text
execution_target text not null
managed_pool_key text null
```

约束：

- `execution_target in ('personal_worker', 'managed_worker')`；
- Personal 时 `managed_pool_key is null`；
- Published Version 不允许修改 Target；
- 同一 `conductor_task_def` 不能同时被 Personal 和 Managed Capability 使用；
- 旧数据迁移为 `personal_worker`，保持当前客户端执行语义。

### 4.2 Executable Workflow IR

Capability IR 必须冻结：

```go
type CapabilityIR struct {
    Key              string
    Version          int
    ConductorTaskDef string
    ExecutionTarget  string
    ManagedPoolKey   string
}
```

Semantic Compiler 拒绝空值、未知 Target、Personal 携带 Pool、Managed 配置非法 Pool。Backend Compiler 不重新查询 Capability，因此重编译和运行使用同一冻结事实。

### 4.3 Conductor Domain

Job 启动时按 IR 生成 `taskToDomain`：

```text
personal_worker -> user:<opaque user hash>
managed_worker  -> managed:<normalized pool key>
```

Domain 只能由服务端生成，不接受 API/客户端传入。Personal 与 Managed 不互相回退，也不回退到 `NO_DOMAIN`。同一 Workflow 中若相同 `conductor_task_def` 被编译到不同 Domain，发布或启动必须失败。

## 5. Worker 运行模型

### 5.1 Personal Worker

- 通过统一网关访问 `/api/worker/v1/*`；
- 使用绑定 `Worker:<worker_id>` 的 API Token；
- 只能注册和领取 `execution_target=personal_worker` 的 Capability；
- Dispatch 保存 `subject_user_id`、`actor_id`、`credential_id`、`worker_instance_id` 和 `auth_source=api_token`。

### 5.2 Managed Worker

- 是 `media_workflow` 服务边界内的独立运行入口；
- 不调用公开 Worker API，不申请 API Token，不访问 Talos；
- 只轮询配置 allowlist 中的 Managed Pool/Capability；
- Dispatch 保存 `requested_by=<job user>`、`actor_id=Worker:<configured id>`、空 `credential_id` 和 `auth_source=internal_runtime`；
- 与 Personal Worker 共用 lease、attempt、Result Inbox 和 Conductor Adapter 语义。

如果 Managed Worker 将来移出服务信任边界，改用 mTLS/Workload Identity；只有代表 Job User 调用内部资源服务时才签发绑定 dispatch、audience、scope 和 lease 的短期委托 JWT。

## 6. Agent API Token

创建 Agent 后，`media_agent_gateway` 使用稳定幂等键请求 `media_auth.CreateActorAPIToken`。`media_auth` 必须回查 Agent 的 owner、Organization 和状态；Gateway 不接受请求体覆盖 owner，也不保存 Secret。

Agent 创建成功而 Token 申请失败时保留 Agent，返回 `token_provisioning_failed`，允许针对同一 Agent 重试。Agent disabled/deleted 或 owner 关系解除时撤销关联 Token。

Agent 运行时只调用 Agent Gateway。Gateway 验证 API Token、重新检查委托关系，再用 `sub=User + act.sub=Agent` 的内部身份代理目标服务。

## 7. 迁移与兼容

1. 先部署 Capability Target 字段和双 Domain 运行逻辑，不改变现有 Personal Worker API。
2. 部署 Talos 与新 `api_tokens` 表、API 和内部验证接口。
3. 客户端与 Gateway 切换为 API Token/Actor Claim。
4. 已有非 Platform Service Token 在明确迁移窗口换发为 Talos API Token；Secret 无法反推时必须换发，不能静默复制 Hash。
5. 所有调用方完成切换后删除 `service_token` domain、IDL、路由和本地 Hash 验证；Platform Token 保持独立。

兼容期不得把同一个 Secret 同时交给本地 Hash 和 Talos 两套验证器。

## 8. 验收标准

- Capability Target migration 可上下迁移，旧 Capability 均变为 Personal。
- Semantic Compiler 与 IR canonical JSON 包含冻结 Target；非法组合无法发布。
- 混合 Personal/Managed Workflow 生成不同 `taskToDomain`，且无 `NO_DOMAIN`。
- Personal Worker 无法领取 Managed Task，Managed Worker 无法领取 Personal Task。
- Managed Worker 全链路不依赖 Talos/API Token，并正确记录请求 User 与执行 Actor。
- Talos 创建、验证、轮换、撤销覆盖成功、无效、过期、撤销、超时和补偿失败。
- JWT 测试证明 `sub`、`act.sub`、credential 和 instance 不混用。
- Agent Token 申请可幂等重试，Gateway 不持久化 Secret。
- 测试环境为测试 User 签发一把受限 API Token，Secret 只写本地 `.env` 且不提交；HTTP/CLI E2E 可通过统一网关携带 `Authorization: Bearer` 建立直接 User Principal，无需模拟浏览器登录。
- `go test ./...`、`make arch-check` 以及相应 HTTP/CLI E2E 通过。
