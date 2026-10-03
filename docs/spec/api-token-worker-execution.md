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

`media_auth/biz/shared/client/talos` 封装 Talos Admin API，业务层不能依赖 Talos SDK/HTTP DTO。生命周期与恢复接口为：

```go
type Client interface {
    Issue(ctx context.Context, cmd IssueCommand) (IssuedKey, error)
    RecoverIssue(ctx context.Context, cmd IssueCommand) (KeyMetadata, error)
    Verify(ctx context.Context, secret string) (VerifiedKey, error)
    Revoke(ctx context.Context, keyID string) error
    Get(ctx context.Context, keyID string) (KeyMetadata, error)
    RotateWithMetadata(ctx context.Context, keyID string, scopes []string, metadata map[string]any) (IssuedKey, error)
    List(ctx context.Context, actorID, pageToken string) (KeyPage, error)
}
```

HTTP 路径和 DTO 以锁定的 Talos OpenAPI 版本为准。Adapter 必须设置超时、传播 `traceparent`、屏蔽 Secret 日志，并把无效/撤销/过期与上游不可用映射成稳定领域错误。

本地开发使用仓库维护的 Talos OSS 单节点容器与持久 SQLite volume；它属于认证基础设施，默认不随普通编码测试启动。

### 3.1 撤销恢复

`api_tokens.status=revoking` 保存不可丢失的撤销意图；先提交后调用 Talos。调用错误后按同一 Key ID 查询，仅确认 `KEY_STATUS_REVOKED` 才可提交本地 revoked，查询失败或未知状态保持 fail closed。

`biz/workflow/api_token_revocation.go` 在 Talos 已装配时运行：启动即扫描，间隔 10 秒、每批 100 条，ID keyset 分页，每条尝试期限 15 秒。失败条目不能阻塞后页；下一轮重新扫描。后台使用持久化 subject 重放现有撤销能力，不保存原请求、会话或 Secret。多个实例重复撤销安全，终态记录不再被扫描。

`0011_api_token_revocation_recovery` migration 增加 `(id) WHERE status='revoking'` 部分索引，不新增凭证或 Secret 表。

### 3.2 轮换持久化与恢复

`0012_api_token_rotations` 增加 Token rotating 状态，以及 `api_token_rotations(id,token_id FK,old_key_id,actor_id,status,deadline,created_at)`；status 为 pending/recovering/completed/aborted，单 Token 最多一个 pending/recovering。表只保存元数据。

`biz/workflow/api_token_rotation.go` 注入 TxRunner、RotationRepository 与 SecretStore。同步请求先在事务中 CAS active/旧 Key/subject/未过期并提交 rotating + pending，期限 30 秒；外部 Rotate 请求在 metadata 写入 api_token_id、rotation_id、subject_user_id、organization_id。成功后另开事务 CAS pending/未超时/rotating/旧 Key，提交新 Key 映射与 completed，再返回 Secret。外部调用不持有事务。

失败时不返回 Secret，尝试取得 recovering 并清理；客户端取消后的清理使用独立有界 context。后台启动即扫描，间隔 10 秒、每批 100 条，以 ID keyset 遍历 recovering 和已过期限的 pending。每次恢复最多 15 秒。取得恢复后同步提交不能再成功。

清理先 Revoke 旧 Key，错误后 Get 确认其 REVOKED，阻止原子 Rotate 的晚到调用；再按 actor_id 调 Talos List（每页100），仅撤销 metadata 同时匹配 api_token_id、rotation_id 的 replacement。全部确认后，在事务中提交 Token revoked + operation aborted。分页/上游/本地失败保留 recovering；不能重发 Rotate，也不能重放 Secret。completed 操作不参与恢复。

Talos Adapter 增加带 metadata 的 Rotate 和按 Actor 的分页 List，正确转义 AIP filter/URL；列表异常不得被当作完整扫描。Service 的 Rotate 保留归属校验并委托 Workflow；启动 wiring 同时装配同步能力和后台恢复。

### 3.3 创建意图与未知签发结果恢复

`api_token_issuance` Workflow 的实现契约见 [签发与恢复时序](../ddd/media_auth/workflow/0010-api-token-issuance/api_token_issuance.md)。`api_tokens` 增加 issuing/issuance_cleanup、nullable talos_key_id、issuance_deadline、nullable client_request_key；创建者范围内的非终态客户端键唯一。迁移回滚遇到未完成意图必须拒绝，不丢弃清理事实。

`0013_api_token_issuance` migration 的 shape 约束要求 issuing 的 Key 为 NULL、创建/清理状态有 deadline，其他状态有非空 Key。客户端键空值存 NULL，最大200字符。Repository 把驱动 SQLSTATE 23505 及 GORM `ErrDuplicatedKey` 映射为 domain conflict。`CommitIssuance` CAS issuing/NULL Key/deadline，并拒绝空 Key 或非未来有效期；`ClaimIssuanceCleanup` 只接受 cleanup、已超时 issuing 或同步强制清理；`FinishIssuanceCleanup` 只提交 cleanup→revoked，同终态 Key 重复完成幂等。普通 `MarkRevoked` 不能终结创建状态。

代码映射：`biz/domain/api_token/issuance.go` 定义仓库接口，`biz/dal/repo/api_token/issuance.go` 实现本地事实，`biz/workflow/api_token_issuance.go` 协调副作用并持有 TxRunner；`Service.SetIssuer` 装配已完成参数/授权校验的创建入口与撤销恢复入口。`wire_gen.go` 同时装配同步入口和后台循环，Talos 未配置仍无可用 Token 服务。

Service 保留授权/参数校验，委托 Issuer 接口；Workflow 在 Talos 调用前持久化 issuing，使用 Token ID 作为 request_id 和 metadata 的 api_token_id/issuance_id。期限内 CAS 提交 active 后才返回 Secret；失败取得清理状态并进行独立有界补偿。后台启动即运行、每10秒扫描100条、每条15秒，失败不阻塞后页。撤销尚未完成的创建进入相同清理状态，不调用空 Key 的普通撤销。

Talos Client `RecoverIssue` 与正常 `Issue` 分离：必须提供非空 request_id、Actor、api_token_id、issuance_id；响应只有 KeyMetadata，不提供 Secret 字段，即使上游重新签发返回 Secret 也必须丢弃。正常 Issue 仍拒绝缺失 Secret。RecoverIssue 精确校验返回的 Actor 和两个 metadata 标识，异常不能撤销其他凭据；错误不得暴露上游正文、Secret 或异常字段。

恢复先分页枚举 Actor 的匹配 metadata；无匹配时使用同一 request_id 调 RecoverIssue 形成晚到签发屏障，随后撤销得到的唯一 Key并确认终态。Talos 在幂等查询前检查 quota；存在 Key 优先通过 List 定位，无 Key 且重放被拒绝时保持清理状态。不能只 List 后宣布完成，也不能生成新的 request_id 重试签发。已成功 active 的 Token 不参与补偿；重复客户端命令返回 conflict，不重新交付 Secret。

测试分别覆盖 Adapter 无 Secret 重放、新签发 Secret 丢弃、跨 Actor/metadata 拒绝，以及 Workflow 取消/本地提交失败/持久恢复、PostgreSQL CAS与去重、真实 Talos 响应丢失后恢复。必须同时通过 HTTP 与 CLI 生命周期门禁才可完成 P4。

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

#### Personal Worker 身份基础

`media_auth/worker_identity` 是 Personal Worker 稳定身份的权威来源，不是 Worker runtime。`0014_worker_identities` 创建表 `(id,owner_user_id FK,organization_id FK,name,status,created_at,disabled_at?,deleted_at?)`，状态 active/disabled/deleted，owner/Organization 不允许变更。身份模型不保存 API Token、Secret、Capability 列表或进程实例。

领域/代码映射为 `biz/domain/worker_identity`、`biz/dal/model/worker_identity.go`、`biz/dal/repo/worker_identity`、`biz/service/worker_identity`。Service.Create 注入 User active 与 Organization membership 校验；ValidateActor 对精确 Worker ID、active、owner、Organization 校验，错误和查不到均拒绝。API Token ActorValidator 增加 Organization 参数，并在签发、验证和轮换时传递；轮换也重新验证 User/企业状态。startup wiring 只装配 Personal Worker 权威，Agent 仍需独立来源。

状态管理只能从 active 进入 disabled/deleted，disabled 可进入 deleted，同状态幂等；不提供重新激活。停用对外入口必须使用 WorkerAccessRevocation：本地事务同时更新身份和 Token 撤销意图，已有后台循环清理 Talos。并发晚到创建仍需非 active 身份扫描补偿，不能只做一次 bulk update。持久查询/状态基础、公开身份与签发 API、撤销编排/后台、Worker Token 换发和真实运行联调作为独立验收项，未完成入口不能把 P5 标记完成。

#### Personal Worker 管理与签发接口

`idl/media_auth/worker_identity.proto` 定义 WorkerIdentityService，所有RPC同时声明google.api.http与api.*注解：

| RPC | HTTP | 输入/输出 |
|:---|:---|:---|
| CreateWorkerIdentity | POST /api/worker-identities | name、organization_id；201 worker_identity |
| ListWorkerIdentities | GET /api/worker-identities | 当前User全部身份，含历史状态；200 worker_identities |
| GetWorkerIdentity | GET /api/worker-identities/:id | 200 worker_identity，其他Owner/未知404 |
| DisableWorkerIdentity | POST /api/worker-identities/:id/disable | WorkerAccessRevocation；200 success |
| DeleteWorkerIdentity | POST /api/worker-identities/:id/delete | 软删除，同上事务；200 success |
| CreateWorkerApiToken | POST /api/worker-identities/:id/api-tokens | name、scopes[]、expires_at、idempotency_key；201 ApiTokenResponse |

Actor、owner与企业不能由签发请求覆盖。WorkerTokenProvisioning.GetOwned检查active，派生`Worker:<id>`、subject=owner、Organization，再通过受信服务内部CreateCommand调用API Token Create；原通用User Token接口不新增任意Actor字段。五类scope为poll/heartbeat/complete/fail/upload，空值默认全部，非空只接受其子集。查询不返回Secret；错误正文固定安全描述。状态命令在Talos不可用时也保存禁止访问及清理意图，首次创建身份的Service独立于Talos配置，签发未配置返回503。

CLI `media_auth worker-identity create/list/get/disable/delete/create-token` 使用生成typed client；create-token必须传`--env-out`新文件，0600保存Secret，不输出Secret，保存失败撤销新凭据；可指定到期、子scope和幂等键。单元/handler/HTTP网关/CLI各自验收，CLI成功不能代替接口失败分支测试。

Traefik认证服务路由与Oathkeeper普通认证API规则同时包含`/api/worker-identities`，沿用Session或要求api.access的User API Token，不将管理入口加入`/api/worker/v1`规则。网关文件配置变更后重启本仓库网关；与浏览器回归串行。

### 5.2 Managed Worker

- 是 `media_workflow` 服务边界内的独立运行入口；
- 不调用公开 Worker API，不申请 API Token，不访问 Talos；
- 只轮询配置 allowlist 中的 Managed Pool/Capability；
- Dispatch 保存 `requested_by=<job user>`、`actor_id=Worker:<configured id>`、空 `credential_id` 和 `auth_source=internal_runtime`；
- 与 Personal Worker 共用 lease、attempt、Result Inbox 和 Conductor Adapter 语义。

如果 Managed Worker 将来移出服务信任边界，改用 mTLS/Workload Identity；只有代表 Job User 调用内部资源服务时才签发绑定 dispatch、audience、scope 和 lease 的短期委托 JWT。

## 6. Agent API Token

### 6.1 Control 身份事实源

Agent Actor 使用 `Agent:<agent_configurations.id>`，不是现有 `agent_id` 类型标签。`GET /internal/v1/agent-identities/{agent_configuration_id}` 为 Control 内部只读入口，要求内部 Service Token、system role 和非空受信 acting-service ID；不加入公开网关路由。

响应字段为 `agent_configuration_id, actor_id, workspace_id, owner_user_id, organization_id, team_id, workspace_state, owner_authorized`。owner/企业/部门来自 Workspace 数据库事实，owner_authorized 按当前 Team view 权限回查；不返回或解密 encrypted_api_key，不返回 Runtime Context。未知配置或 deleted Workspace 为404，权限服务错误为503，空 ID 为400。停止运行的 Workspace 不等于身份删除，业务执行仍须进行 Workspace/action 授权。该查询自身无写入和跨事务副作用，不新建 Workflow。

实现映射：`media_control/biz/domain/workspace` 身份读模型与查询接口、`biz/dal/repo/workspace` 单次 join 查询、`biz/service/agent_identity` 只读用例、IDL/generated router/薄 handler、main 装配。Auth 接入、Gateway 申请/代理和 disabled/解绑撤销是独立后续门禁，不以该查询上线代表 P6 完成。

Auth 的 `biz/shared/client/control` 封装生成的 AgentIdentityService client，只向配置的内部地址发送固定 system 服务身份，不传用户 Cookie 或 API Token；5 秒总超时且不超过父请求期限，查询取消/错误/非200均拒绝。禁止把生成客户端可能包含来源响应正文的错误传给业务层，禁止缓存身份授权。配置为 `MEDIA_CONTROL_URL` 与 `CONTROL_INTERNAL_TOKEN`；缺少任一项时 Agent Validator 不装配，仍 fail closed。

`biz/service/api_token/AgentActorValidator` 精确匹配 Configuration ID、`Agent:<id>`、owner、企业、非空 Workspace/Team 及 owner_authorized，拒绝 deleted/未知 Workspace 状态。现有 Create/Verify/Rotate 统一调用 ActorValidator，Personal Worker 继续使用独立的 Worker Identity 权威，其他未知 Actor 类型不得借 Agent 校验通过。此接线不开放任意客户端提交 Actor 的签发 API，也不代表生命周期后台撤销或 Gateway 代理完成。

### 6.2 签发与代理

Gateway 显式申请接口：`POST /api/v1/agent-clients/:agent_configuration_id/api-tokens`，请求 `name,expires_at,idempotency_key`，响应201 `CreateApiTokenResponse`。路由仅允许当前 User；Gateway 验证 Oathkeeper `internal-api` JWT 并调用 Auth GetMe 解析/检查当前 User。API Token 分支必须是直接 User、含 `api.access`/credential_id，保留 organization_id；浏览器使用 Auth 的 Kratos 身份解析。非法或失败的 Authorization 不能退回 Cookie。

`biz/workflow/agent_token_provisioning.go` 回查 Control 身份，核对配置/Actor/owner/Organization/Team/Workspace/OwnerAuthorized；API Token 限制的企业必须精确匹配。`biz/shared/client` 用生成客户端封装 Auth GetMe、Control Identity 和 Auth 私有签发，固定服务凭据/地址、总期限与 Trace Context，屏蔽上游敏感错误，签发不自动重试。handler 只绑定参数、调用 Workflow、转换响应，返回 no-store。Auth 私有401视作配置不可用503；其他安全400/403/404/409按契约映射。

配置 `INTERNAL_JWKS_URL,INTERNAL_JWT_ISSUER,MEDIA_AUTH_URL,MEDIA_AUTH_AGENT_GATEWAY_TOKEN,MEDIA_CONTROL_URL,CONTROL_INTERNAL_TOKEN`；签发专用凭据默认空，未配置不能启用。公开 Traefik 路由仅转发该管理 API；内部签发仍不公开。新增 CLI `media_agent_gateway agent-client create-token`，Secret 只写显式新文件0600，控制台只输出元数据；未知HTTP结果不得隐式重试或轮换。该入口可在配置创建后调用，自动创建编排与运行时代理单独验收。

Auth 提供 `POST /internal/v1/agent-api-tokens`（CreateActorAPIToken）：内部请求体仅含 `agent_configuration_id,name,expires_at,idempotency_key,scopes[]`。可信调用者 User 从 `X-Media-Auth-Acting-User-ID` 取得，调用方必须提供专用 `X-Agent-Gateway-Internal-Token`，与 `MEDIA_AUTH_AGENT_GATEWAY_TOKEN` 常量时间比较；未配置时拒绝，不复用一般内部查询凭证。

AgentTokenProvisioning 回查 Control，检查 stable Actor/config ID、当前 owner/企业/部门权限和未删除配置，要求可信 User 等于事实 owner。Subject/Actor/Organization 只由权威事实派生；第一阶段 scope 只允许 `agent.gateway`（空数组默认该 scope），拒绝 Worker 和直接 `api.access` scope。幂等键由配置 ID、固定 purpose 和调用方请求键构成有界摘要，调用方请求键必须非空且不超过200字符。

成功复用 API Token Issuance，返回201 ApiToken Metadata和仅一次 Secret；非法参数400，归属/授权不符403，未知配置404，重复签发409且不重放Secret，服务未装配/来源不明/持久签发失败503。成功后连接丢失不能重交付Secret，也不能以同一命令自动轮换；用户通过显式轮换/重新申请恢复。Gateway 侧申请/回传与运行时代理另行实现，Auth 内部接口不加入公开网关路由。

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
