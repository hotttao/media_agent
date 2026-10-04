# E2E 测试

## 测试分工

| 测试类型 | 位置 | 驱动方式 | 覆盖 |
|---------|------|---------|------|
| Go HTTP E2E（需启动服务、打 HTTP） | `media-cli/e2e/` | `go test` + 裸 HTTP + 进程内 cookie jar（被测服务由 docker-compose 起） | 可重复执行的接口断言、异步收敛和错误分支 |
| media-cli 命令 E2E | `media-cli/cmd/<service>/` | 连续执行 `media-cli <service> <command>`，使用落盘 Cookie 跨进程复用登录态 | CLI 命令树、参数映射、生成 client、中间件和真实用户操作链路 |
| 接口无关测试（domain 逻辑/算法/迁移） | `<app>/e2e/` | go test + testcontainers | domain 业务逻辑、事务、纯算法 |

Go HTTP E2E 与 media-cli 命令 E2E 是两道独立门禁，不能互相替代：`media-cli/e2e/` 中的测试通过，不代表对应 CLI 子命令存在或可用；CLI 命令执行成功，也不能替代 Go 测试对失败、幂等和并发分支的断言。

## 启动流程（4 步）

```bash
# 1. tearup 基础服务（按需起 infra 子集，dev=只起 pg，local=全部）
INFRA=dev  docker compose -f docker-compose/docker-compose.yml up -d postgres
# 或全部 infra：
INFRA=local docker compose --profile infra -f docker-compose/docker-compose.yml up -d

# 2. tearup 被测试服务
INFRA=local docker compose -f media_auth/docker-compose.yml up -d media_auth

# 3a. 执行 Go HTTP E2E
MEDIA_AUTH_ADDR=http://localhost:8001 \
  go -C media-cli test ./e2e -run TestAuthE2E -count=1 -v

# 3b. 执行 media-cli 命令 E2E（cookie 跨进程复用）
MEDIA_AUTH_ADDR=http://localhost:8001 \
  go -C media-cli run . media_auth auth login --username alice --password secret123
MEDIA_AUTH_ADDR=http://localhost:8001 \
  go -C media-cli run . media_auth auth me
# 或 service 特有子命令（如 media-cli media_example 自己的命令）

# 4. teardown 被测试服务
docker compose -f media_auth/docker-compose.yml down
# 基础服务保留常驻（除非确定不再测试）
```

## docker-compose profiles 与 env_file

### 基础环境（`docker-compose/docker-compose.yml`）

- `postgres` 无 profile（默认启，单元测试/单服务调试必需）
- `otel-collector`/`jaeger`/`prometheus`/`loki`/`grafana`/`promtail`/`consul` 标 `profiles: [infra]`（默认不启，联调时按 profile 启）

### 切换 infra 子集

```bash
# 单元测试/单服务调试：只起 pg（INFRA=unit 用 .env.unit，关其他 infra）
INFRA=unit  docker compose -f docker-compose/docker-compose.yml up -d postgres
# 联调全部：起 pg + 所有 infra（INFRA=e2e 用 .env.e2e，开所有）
INFRA=e2e   docker compose --profile infra -f docker-compose/docker-compose.yml up -d
# 联合启动（基础 + 服务）：
INFRA=e2e   docker compose -f docker-compose/docker-compose.yml -f media_auth/docker-compose.yml up -d
```

### env_file 共享（`docker-compose/.env.<infra>`）

- `.env.unit`：只开 pg（其他 infra=false）
- `.env.e2e`：开全部 infra

被测服务 compose 引用共享 env_file：
```yaml
services:
  media_auth:
    env_file:
      - ../docker-compose/.env.${INFRA:-e2e}   # INFRA 切换 → 自动选 unit/e2e
    environment:
      SERVER_ADDR: ":8001"                    # 服务特有变量（不随 INFRA 变）
      MEDIA_AUTH_DB_DSN: postgres://...@host.docker.internal:5432/media_auth?sslmode=disable
      LOG_DIR: /media_logs/media-auth
    volumes:
      - ../media_logs/media-auth:/media_logs/media-auth   # 日志映射 host（grep debug）
    extra_hosts:
      - "host.docker.internal:host-gateway"             # 容器内访问 host pg
```

## 文件组织

```text
docker-compose/
  docker-compose.yml              # 基础环境（pg + infra services with profiles）
  .env.unit                         # INFRA=unit：只开 pg
  .env.e2e                          # INFRA=e2e：开全部 infra
media_auth/
  Dockerfile
  docker-compose.yml                # media_auth 独立 compose（build + env_file + ports + volumes）
media_example/
  Dockerfile
  docker-compose.yml                # 同上
media-cli/
  cmd/<service>/                    # media-cli 命令 E2E 入口（落盘 cookie 跨进程复用）
  e2e/                              # Go HTTP E2E（裸 HTTP + 进程内 cookie jar）
```

## 关键约定

### 通过统一网关使用 API Token

Ory 认证环境的接口测试使用预先签发的 API Token。业务服务接收网关交换的内部 JWT；测试应请求网关地址。
当前本地测试凭据保存在未跟踪的 `ui_workflow/e2e/.env` 的 `E2E_API_TOKEN` 中。

```bash
set -a
. ui_workflow/e2e/.env
set +a
export MEDIA_API_TOKEN="$E2E_API_TOKEN"
export MEDIA_GATEWAY_ADDR=http://192.168.2.41:18080
export MEDIA_AUTH_ADDR="$MEDIA_GATEWAY_ADDR"
export MEDIA_E2E_ORGANIZATION_ID=8afeb26b9df643952f4f816576eb2504

# HTTP 生命周期、scope 与组织隔离测试（不模拟登录）
go -C media-cli test ./e2e -run '^TestAPITokenGateway$' -count=1 -v

# CLI 跨进程调用（MEDIA_API_TOKEN 优先于 Cookie）
go -C media-cli run . media_auth auth me
go -C media-cli run . media_auth api-token list
```

CLI 提供 `api-token create/list/rotate/revoke`。创建和轮换要求 `--env-out` 指向新文件，Secret 只写该文件（0600），控制台只返回元数据。测试不会输出 Secret；`.env` 不得提交。旧的本地 Session 测试仍适用于没有启用 Ory 的独立服务环境。

### Personal Worker 身份与撤销集成测试

`media_auth/e2e/worker_identity*.go` 使用隔离 PostgreSQL，覆盖归属、状态 CAS、事务回滚、Actor/Organization 隔离、晚到签发恢复和降级迁移保护。真实 Talos 测试创建唯一临时 Worker Actor，验证停用事务提交后立即拒绝访问，重建撤销 Workflow 后 Talos Key 进入 revoked；只撤销测试自己创建的 Key，不使用或改动已有 Worker 凭据。

```bash
worker_talos_ip=$(docker inspect --format '{{(index .NetworkSettings.Networks "media-auth").IPAddress}}' media-agent-auth-talos-1)
MEDIA_E2E_TALOS_URL="http://${worker_talos_ip}:4420" \
  go -C media_auth test -tags=e2e -race ./e2e -run '^TestWorkerIdentity' -count=1 -v
```

这组测试不替代后续公开身份/签发接口、CLI和真实media_worker执行验收。未配置Talos时对应测试明确Skip；普通单测不连接Talos。

公开接口门禁为`media-cli/e2e/worker_identity_e2e_test.go`：使用User API Token访问网关创建/读取/查询/签发/停用/删除，验证客户端不能覆盖owner/Actor/Organization、scope限制、重复签发不交付Secret、Worker JWT交换、管理API隔离、停用即时拒绝及后台撤销收敛。未消费Conductor任务，不替代真实Worker单流程验收。

```bash
go -C media-cli test ./e2e -run '^TestWorkerIdentityGateway$' -count=1 -v

# Worker Gateway 注册、scope 与撤销边界；不领取已有任务
go -C media-cli test ./e2e -run '^TestWorkflowWorkerGatewayAPIToken$' -count=1 -v
```

环境变量同上方User API Token网关测试。CLI另用`media_auth worker-identity create/list/get/create-token/disable/delete`验收，Token Secret用`--env-out`新文件0600交付，禁止输出；完整命令链和HTTP断言是两道独立门禁。

Worker Gateway 边界测试为现有企业创建临时稳定 Worker 身份和 poll-only API Token，通过网关验证有效注册、User 不能冒充 Worker、scope 不足拒绝，以及撤销后注册/轮询立即拒绝。只清理本次创建的身份和凭据；不模拟登录，不调用旧 Service Token 接口，不用有效 Token 领取队列任务，不替代完整 Worker 执行门禁。

### Personal Worker 单样本执行门禁

真实 Personal Worker 单流程门禁需要先按 `media_worker/README.md` 启动独立客户端，再显式执行；测试本身不模拟 Worker，不启动服务，不输出凭据或签名资源 URL：

```bash
MEDIA_E2E_PERSONAL_WORKER_SAMPLE=1 \
MEDIA_E2E_SAMPLE_WORKFLOW_ID=<published-four-step-sample-id> \
go -C media-cli test ./e2e -run '^TestWorkflowPersonalWorkerFixture$' -count=1 -v
```

沿用上文网关/User API Token/Organization 环境变量。当前用户有未结束 Job 时拒绝提交。失败/超时保留执行供排查，不能自动重提；使用 `MEDIA_E2E_SAMPLE_JOB_ID=<existing-job-id>` 仅验收已有执行。样本完成后须用户确认，再迁移其他 Workflow 数据。

### 历史发布记录只读验收

使用上文 User API Token 和统一网关环境变量，指定已导入记录及 Workspace。该测试读取真实成员 Job，实际下载样本媒体并重算 SHA-256，检查未认证访问及 Workspace 隔离；不创建 Job、不启动 Worker。

```bash
MEDIA_E2E_WORKSPACE_ID=2e6c5e63-b343-47c4-9f4e-64eb3c7855ce \
MEDIA_E2E_HISTORICAL_PUBLICATION_ID=hvp_7e4bb02a-2345-4d9f-b861-38582c8784db \
go -C media-cli test ./e2e -run '^TestHistoricalPublicationGateway$' -count=1 -v

MEDIA_WORKFLOW_ADDR="$MEDIA_GATEWAY_ADDR" \
go -C media-cli run . media_workflow historical-publication get --id <record_id>
MEDIA_WORKFLOW_ADDR="$MEDIA_GATEWAY_ADDR" \
go -C media-cli run . media_workflow historical-publication list --workspace-id <workspace_id>
```

全量迁移工具的 `--verify-report` 是单独的只读对账门禁，须覆盖每个来源/目标及存储对象，不以本单样本测试代替。

### 关闭观测后端的 HTTP 请求关联

`TestRequestCorrelationGateway` 使用同一套网关/User API Token 配置，只读验证 Workflow/Materials API 保留合法 W3C trace ID，无父上下文时返回新的 `X-Trace-ID`。测试不启动观测后端；落盘日志对应关系另以服务日志核对，不用本测试证明异步 Worker/Outbox/Conductor 链路。

```bash
go -C media-cli test ./e2e -run '^TestRequestCorrelationGateway$' -count=1 -v
```

### Agent 稳定身份只读查询

`TestAgentIdentityControl` 使用网关 User API Token 读取现有 Workspace，再直连 Control 内部身份入口核对稳定配置 ID/Actor/归属、服务身份保护和秘密字段不泄露；不创建 Workspace/Agent/Token，不启动 Runtime。需要额外设置 `MEDIA_E2E_WORKSPACE_ID`、`MEDIA_CONTROL_ADDR`（Control 内部地址）及 `MEDIA_CONTROL_INTERNAL_TOKEN`（从私有环境配置取得，禁止输出）。该内部入口不应暴露在公开 Traefik 路由中。

```bash
go -C media-cli test ./e2e -run '^TestAgentIdentityControl$' -count=1 -v
```

公开 CLI `media_control workspace get --workspace-id <id>` 单独验证稳定 `agent_configuration_id` 引用。该门禁不证明 Agent Token 签发、代理或撤销已经实现。

### Agent API Token 私有签发

`TestAgentTokenInternalProvisioning` 通过真实 Auth/Control/Talos 签发临时 Agent Token，检查归属派生、专用服务身份、scope、重复请求不重放 Secret、公开列表及撤销后的验证。测试只创建/撤销自己的凭据，不启动 Agent Runtime、不修改 owner，也不消费 Conductor 队列。

除上文 User API Token/网关/Workspace 配置外，需要 `MEDIA_AUTH_INTERNAL_ADDR`（Auth 私有地址）、`MEDIA_AUTH_AGENT_GATEWAY_TOKEN`（专用签发凭据）和 `MEDIA_E2E_AUTH_INTERNAL_TOKEN`（内部验证凭据）。秘密只从私有环境文件加载，不输出、不提交；该内部签发路径不能暴露到公开 Traefik。当前专用本地文件为 `media_auth/.env.agent-gateway.local`，已被 Git 和 Docker 构建上下文排除，权限为 0600。

```bash
go -C media-cli test ./e2e -run '^TestAgentTokenInternalProvisioning$' -count=1 -v

# 独立 CLI 管理门禁；仅使用本次测试创建的 Token ID
go -C media-cli run . media_auth api-token list
go -C media-cli run . media_auth api-token revoke --id <test_token_id>
```

此门禁不证明 Agent Gateway 的申请/代理或停用/删除/解绑触发的批量持久撤销已经完成；不为内部签发增加公开 CLI 绕过 Gateway。

### Gateway 已有 Agent 配置申请

`TestAgentTokenGatewayProvisioning` 全程使用 User API Token 和公开统一网关，不依赖内部签发凭据。设置上文网关/Workspace变量，测试仅申请并撤销自己的临时 Agent Token，验证身份字段/scope伪造、未知配置、重复命令、no-store及管理入口隔离。它不创建或启动 Agent Runtime。

```bash
go -C media-cli test ./e2e -run '^TestAgentTokenGatewayProvisioning$' -count=1 -v

MEDIA_AGENT_GATEWAY_ADDR="$MEDIA_GATEWAY_ADDR" \
go -C media-cli run . media_agent_gateway agent-client create-token \
  --agent-configuration-id <configuration_id> --name <test_name> \
  --expires-at <RFC3339> --idempotency-key <unique_request_key> \
  --env-out <new_private_file>
```

CLI 是独立门禁：核对新文件0600、管理列表中的 Agent/owner/企业/scope，并使用 User 管理 API 撤销本次 Token。该门禁只覆盖已有配置显式申请，不证明自动创建编排、Agent运行时代理或生命周期批量撤销已完成。Traefik当前关闭文件热加载，路由配置变更需要显式重启统一网关。

### Agent 受限业务代理

`TestAgentProxyGateway`使用User API Token申请临时Agent Token，经公开网关读取任务并创建/读取/删除一条临时纯文本素材，检查目标allowlist、身份类型、企业隔离、trace_id及撤销后拒绝。只撤销本次凭据、删除本次素材，不提交Workflow、不启动Runtime/Worker。沿用网关/User Token/Workspace环境变量：

```bash
go -C media-cli test ./e2e -run '^TestAgentProxyGateway$' -count=1 -v
```

CLI门禁先用User执行`agent-client create-token --env-out <new_private_file>`，核对0600后仅在Agent调用进程加载其中的`MEDIA_API_TOKEN`：

```bash
MEDIA_AGENT_GATEWAY_ADDR="$MEDIA_GATEWAY_ADDR" \
go -C media-cli run . media_agent_gateway agent-client request \
  --service workflow --method GET --uri '/api/v1/jobs?page_size=1'
```

再用原User凭据执行`media_auth api-token revoke --id <this_test_token_id>`，确认Agent请求失败；不要以Agent Token调用管理API。代理只发往固定服务/业务路由，不提供公开内部JWKS命令。该门禁不证明自动创建编排、生命周期批量撤销或完整审计落盘已完成。

### API Token 轮换恢复集成测试

`media_auth/e2e/api_token_rotation_test.go` 使用 testcontainers 的隔离 PostgreSQL；设置 `MEDIA_E2E_TALOS_URL` 后还会连接已由仓库启动的 Talos Admin API。测试为唯一临时 User Actor 创建 Key，不使用测试账号的既有凭据；结束后撤销自己创建的 Key。

```bash
rotation_talos_ip=$(docker inspect --format '{{(index .NetworkSettings.Networks "media-auth").IPAddress}}' media-agent-auth-talos-1)
MEDIA_E2E_TALOS_URL="http://${rotation_talos_ip}:4420" \
  go -C media_auth test -tags=e2e -race ./e2e -run '^TestAPITokenRotation' -count=1 -v
```

这个测试让真实 Talos 提交轮换，然后在 transport 层丢弃响应并模拟清理不可用；重建 Workflow 后只用持久化操作恢复，验证旧 Key 与未知 replacement 均 revoked、不会重发 Rotate、已完成操作不被补偿。普通 `go test ./...` 不会连接 Talos。此测试与浏览器 E2E 串行执行。

### API Token 签发恢复上游契约测试

`media_auth/e2e/api_token_issuance_test.go` 为唯一临时 Actor 验证真实 Talos 的 request_id 重放规则，不使用既有测试账号的 Key：签发提交后丢弃响应，恢复只取得元数据；恢复先于原请求到达时也仅创建一个 Key；撤销后重放不能复活凭据，正常 Issue 不能再次取得 Secret。

```bash
issuance_talos_ip=$(docker inspect --format '{{(index .NetworkSettings.Networks "media-auth").IPAddress}}' media-agent-auth-talos-1)
MEDIA_E2E_TALOS_URL="http://${issuance_talos_ip}:4420" \
  go -C media_auth test -tags=e2e -race ./e2e -run '^TestAPITokenIssuanceTalosRecoveryContract$' -count=1 -v
```

未设置地址时该测试明确 skip。它只证明 Adapter 与锁定 Talos 的契约，不替代创建 Workflow 的数据库持久意图/CAS/重启恢复测试，也不替代 HTTP 与 CLI 生命周期门禁。

`api_token_issuance_repo_test.go` 使用隔离 PostgreSQL 验证 nullable Key、创建者范围内并发去重、激活/清理 CAS、事务回滚、过期扫描、未完成创建的撤销和 migration 回滚保护。`TestAPITokenIssuanceRealTalosLostResponseSurvivesRestart` 使用真实 Talos 提交后丢弃响应并阻断即时清理，重建 Workflow 后仅用数据库意图清理。统一运行 `-run '^TestAPIToken'` 同时覆盖创建、撤销和轮换；普通单元测试不连接这些服务。

- **`media-cli` 不用管服务启停**——那是 docker-compose 的事。media-cli 只发 HTTP 请求。
- **两个 E2E 层级都要验收**——功能存在用户/管理 HTTP 入口时，既要运行 `media-cli/e2e` 的 Go HTTP E2E，也要实际执行对应 media-cli 命令链路；纯内部事件入口可以由面向用户的 CLI 流程间接覆盖，不要求暴露危险的内部命令。
- **基础环境常驻**（除非确定不再测试才停），避免每次 e2e 重复 build/起。
- **cookie 跨命令复用**：`media-cli media_auth auth login` 下发 cookie 落盘 `media-cli/.cookies/`，后续 `auth me`/`change-password` 自动带。
- **日志映射 host**：`LOG_DIR=/media_logs/<app>` + volume 映射 `../media_logs/<app>/`，便于 `grep` debug。

## 边界

- 不再提供 media-cli `e2e` 编排子命令（用 docker-compose 直管，更灵活）
- 接口无关测试（migration / 事务 / 纯算法）继续用 `<app>/e2e/` 的 go test + testcontainers
