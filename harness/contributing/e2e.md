# E2E 测试

## 原生 rerun 与真实本地交付时间边界

`media-cli/e2e/workflow_boundary_probe_test.go` 必须显式设置 `MEDIA_E2E_BOUNDARY_PROBE=1`、已核对的 `MEDIA_E2E_BOUNDARY_JOB_ID` 和 User/Worker API Token。Submit 单独调用 `TestWorkflowBoundaryProbeSubmit`，先审计整个 User 队列，只接受一个固定键部分执行。协议门禁 `TestWorkflowNativeRerunAndLeaseBoundaryGateway` 使用真实 HTTP，明确失败首节点后执行 input_override 整次原生 rerun，检查固定版本输入审计与原 trace，分别等待真实 90 秒时间过期再发心跳/Complete；不改数据库租约，不在测试内启动模型 Worker，不审批，结束后取消原实例。

失败后先只读核对精确实例；上游可能已经提交，不能直接重跑 Submit/rerun 掩盖未知结果。该门禁约需 200 秒，超时应至少 5 分钟。取消后的同一测试 Job 不再运行整套门禁，也不能把部分执行当成第二条完整模型验收。Token、提示词、完整输入和签名 URL 均不得输出。

## 异步trace与原生状态部署复验

`media-cli/e2e/workflow_trace_probe_test.go` 的Submit门禁显式使用`MEDIA_E2E_TRACE_PROBE=1`和已有完整样本`MEDIA_E2E_TRACE_SOURCE_JOB_ID`；核对Owner/企业和整个User队列，刷新稳定对象引用，仅接受固定键的一个部分执行，不启动Worker/不审批。结果记录Job ID与非秘密trace_id，禁止输出Input、Token和签名URL。

对已核对的`MEDIA_E2E_TRACE_JOB_ID`/`MEDIA_E2E_TRACE_ENGINE_ID`，`TestWorkflowTraceProbeNativePauseResumeProjection`仅对原实例执行Conductor原生pause/resume并检查公开Job投影；失败时只尝试恢复自己的PAUSED实例。必须结束暂停后再准备Provider文件配置，不并行这些依赖步骤。配置准备后在测试进程外启动独立客户端，只注册图像能力。

`TestWorkflowTraceProbeCancelAtFirstApproval`等待首节点两候选可访问，只发送取消/同取消重放，再查询原实例TERMINATED，不确认或执行后续节点。随后停止该客户端；API、后台启动、内部Renderer、客户端回写及Materials日志须具有同一trace_id，不能只以X-Trace-ID响应头证明跨服务。只读审计启动记录cancelled、Generated Asset零新增与Inbox零新增。该部分执行不是第二条完整Workflow、真实Provider或历史恢复验收。

## Worker本地文件产物上传

Provider单样本流程沿用下文独立CLI Submit/外部Worker门禁。配置准备时设置`MEDIA_E2E_PROVIDER_FILE_RESULTS=1`：只下载已知真实结果为新私有文件，配置使用`result_files`，不在测试进程内执行Worker或预上传本次结果。必须先核对整个User待执行队列；仅Submit一个Job，客户端只调用网关与签名Storage PUT，不访问Conductor。

`TestProviderCapabilityExecution`继续执行审批/反馈循环、显式失败和CLI原生retry，`TestWorkflowCompletedOutputsGateway`在文件模式下增加完整对象SHA256与本地结果逐一比较。`TestWorkerArtifactUploadGatewayRejectsInvalidAdmission`使用已有完成Dispatch，验证无Token/User身份拒绝和原Worker410；只提供环境中的Worker凭证，不打印Token/签名URL，不创建对象或新任务。

决策接受是异步的，不把仍未更新的首轮缓存当作第二轮结果。当已确认regenerate后门禁退出，明确核对原生决定并设置`MEDIA_E2E_PROVIDER_REGENERATION_ALREADY_REQUESTED=1`继续同一个样本，不重发regenerate或Submit。仅适用于已核对该决定的恢复观察，不用于掩盖未知结果。技术签名PUT单测/真实Seaweed协议、公开客户端业务流程分别记录。

## Provider失败分类与原生自动retry

```bash
MEDIA_E2E_CONDUCTOR_URL=http://127.0.0.1:8080 \
go -C media_workflow test -tags=e2e -race ./biz/workflow/worker_gateway \
  -run '^TestProviderFailureClassificationNativeRetry$' -count=1 -v -timeout=2m

go -C media_workflow test -tags=e2e -race ./e2e \
  -run '^TestProviderFailureDirectProxyPersistence$' -count=1 -v -timeout=2m
```

第一项使用真实Conductor、唯一技术定义/实例与受控业务端口，通过直接fail/complete验证临时故障自动retry与永久故障不retry，不重置failed缓存。第二项使用隔离PG及明确引擎桩，验证分类/终态重放、分类冲突、Dispatch记账、Inbox零新增与Job不变。仅清理测试资源，不代替公开网关/独立CLI/客户端完整验收。

## Conductor原生失败恢复（技术门禁）

```bash
MEDIA_E2E_CONDUCTOR_URL=http://127.0.0.1:8080 \
go -C media_workflow test -tags=e2e -race ./e2e \
  -run '^TestConductorNativeRetryCompiledParallelWorkflow$' -count=1 -v -timeout=2m
```

实际编译器生成FORK_JOIN/DO_WHILE/SWITCH图，仅将SIMPLE任务名称别名到测试独享队列，不修改引用、表达式或图。完成B旁支、失败A调用后POST原生retry，保留B全部Task身份/状态/输入输出和A既有render，继续A调用、等待JOIN异步decider、合流完成。不使用本地重置、execution_retries、恢复Task或手工重排队。

bounded poll为空不判断失败；继续同一实例到终态/超时。测试只创建/清理自身独立定义/实例，不连接用户数据库，不执行真实模型。技术门禁不替代公开Gateway/独立CLI/真实客户端Worker的完整业务验收。缺Conductor配置明确Skip。旧持久重试命令方案已按用户执行边界废弃，测试代码备份撤出，不再作为当前门禁。

## 原生Task交付不受失败缓存阻塞

```bash
MEDIA_E2E_CONDUCTOR_URL=http://127.0.0.1:8080 \
go -C media_workflow test -tags=e2e -race ./biz/workflow/worker_gateway \
  -run '^TestNativeTaskDeliveryIgnoresFailedProjection$' -count=1 -v -timeout=2m
```

门禁创建唯一技术定义/实例，本地Job始终缓存failed。代理在真实Task更新已提交后丢弃响应，Gateway查询同Task确认，再在lease过期后重放同结果，只有一次POST且Inbox零写入。不callback、不重置；只清理测试资源，不调用模型/用户库，不替代公开HTTP/独立CLI门禁。

## Provider Capability纠正单样本

`media_ai_workflow_migrate`显式提供`--image-capability jimeng.i2i.multi_image_prompt@1 --video-capability doubao.i2v.first_frame_prompt@1`，创建独立模板名称和新发布Workflow，不改旧模板/Compilation。Worker使用`provider_fixture`，不得继续拿业务分类Capability的旧fixture冒充底层Provider契约。

先完整审计待执行队列并通过独立CLI只提交一个Job，然后设置现有网关/User API Token/企业、`MEDIA_E2E_SAMPLE_JOB_ID`及`MEDIA_E2E_PROVIDER_SAMPLE=1`：

```bash
MEDIA_E2E_PROVIDER_FIXTURE_CONFIG=/absolute/private/new-fixture.json \
go -C media-cli test ./e2e -run '^TestProviderCapabilityFixtureConfig$' -count=1 -v
```

该门禁只读取已确认Job、模板版本/Capability与受控媒体，确认队列只有此样本活跃，创建新的0600配置（禁止覆盖已有文件）；不Submit、不启动Worker。签名URL不输出，文件只放私有目录，启动前须保持新鲜。之后在测试进程外按`media_worker/README.md`显式启动真实客户端，凭据为Worker Actor的API Token，不是User Token。

```bash
MEDIA_E2E_CLI_BINARY=/absolute/path/to/media-cli \
go -C media-cli test ./e2e -run '^TestProviderCapabilityExecution$' -count=1 -v -timeout=3m
```

Execution用HTTP观察/请求regenerate及同键重放，通过独立CLI进行失败Step Retry和confirm（预期model两候选、失败序号2）；整个测试不提交另一个Job，也不在测试中实现或启动Worker。结束后停止客户端fixture，后端常驻。成功节点的图片/视频另做无API Token的Storage Range206读取。

首次自动重试若因现存交付窗口失败，必须保留失败证据并核对原实例，不自动修复或重发Submit。`MEDIA_E2E_PROVIDER_CONTINUE_AFTER_RETRY=1`仅观察已确认决定/Retry的同一Job，不再发送regenerate/retry，也不自动重排队；受控管理恢复必须在报告中单列。2026-10-06本次唯一样本首轮失败、人工回收一条未交付任务后恢复成功，不能据此关闭可靠Retry门禁；详见[验收记录](../../docs/review/media_workflow/workflow/0003-conductor-compilation/provider-capability-acceptance.md)。

发布恢复公开门禁 `TestWorkflowPublicationGateway` 与独立命令门禁 `TestWorkflowPublicationCLI` 位于 `media-cli/e2e/workflow_publication_test.go`。预置 `MEDIA_GATEWAY_ADDR`、`MEDIA_API_TOKEN`、授权的 `MEDIA_E2E_PUBLICATION_SOURCE_ID`；CLI另需绝对路径 `MEDIA_E2E_CLI_BINARY`。两者只复制源Workflow为唯一测试定义、发布并查询收敛，最后归档自身资源；不启动Job、Worker或模型，不改借用源。HTTP验证8路同版本单回执/202/完成重放/归档拒绝；CLI真实独立进程发布与查询，不能用HTTP测试替代。Conductor网络失效/进程重启另由Workflow隔离库故障门禁证明。

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

Materials公开门禁 `TestMaterialsE2E` 和上传门禁 `TestLibraryObjectUploadE2E` 使用同一预置API Token，不需要Cookie、密码或 `MEDIA_MATERIALS_ADDR`。三类资源测试只清理自己创建的资源，不撤销借用凭据；上传使用可解码的真实2×2 PNG，核对大小/类型/SHA-256、同内容重试路径/校验和相同、不同内容409，再通过公开解析接口取得短时URL，由不带API Token/Cookie且不跟随重定向的独立客户端下载、逐字节比较并解码。另验证跨企业上传及解析均被拒绝。`TestLibraryPNGFixtureIsValidAndDistinct` 为本地夹具有效性门禁，不代替真实上传验收。上传对象保留在测试企业对象存储中，本轮没有对象删除API，不直接绕过服务删除底层数据。

```bash
go -C media-cli test ./e2e -run '^(TestMaterialsE2E|TestLibraryObjectUploadE2E)$' -count=1 -v
```

`TestMaterialsReachability` / `TestMaterialsBindingsEndAt` 保留Binding、按owner解析和绑定失效断言；这两项在当前里程碑会因未实现Binding路由而失败，不能以CRUD绿色冒充覆盖。前者另需 `MEDIA_MATERIALS_PRIVATE_ADDR`，私有可达性路径使用IDL规定的冒号，不经公共网关；服务的禁用中间件仍生效，不绕过。`TestMaterialsGenPublishIntegration` 另需预启动的 `MEDIA_GEN_ADDR` 以及测试企业已有的真实 `MEDIA_E2E_VIDEO_OUTPUT_URL`，缺配置为Skip，不是发布验收通过。创建唯一来源Product/生成记录，清理测试记录，不删除借用视频对象。旧Workflow测试的凭据已切换，但旧Template/claim等协议仍须单独适配当前Compiler/Worker契约，不代表完整执行验收。

### Platform Token 独立密文存储集成测试

旧验证退役门禁 `TestLegacyTokenVerificationRetirement` 通过三个服务的私有 HTTP 地址核对已移除的RPC返回404、旧Bearer拒绝及Session/API Token内省仍可用。被测服务预先启动；测试不读取旧Secret、不连接数据库、不模拟登录：

```bash
MEDIA_AUTH_PRIVATE_ADDR=http://<Auth-private-IP>:8001 \
MEDIA_WORKFLOW_PRIVATE_ADDR=http://<Workflow-private-IP>:8002 \
MEDIA_MATERIALS_PRIVATE_ADDR=http://<Materials-private-IP>:8003 \
MEDIA_AUTH_INTERNAL_TOKEN=<configured-private-introspection-credential> \
  go -C media-cli test ./e2e -run '^TestLegacyTokenVerificationRetirement$' -count=1 -v
```

凭据从忽略的env文件注入，不把实际值写在命令记录中。此门禁不代替网关API Token成功流程、独立CLI门禁或旧CRUD/归档清理验收。各服务中间件单测另使用有效Cookie验证失败Bearer及大小写/其他Authorization变体均不得降级，且不会向旧Auth请求身份。

`TestLegacyTokenCRUDRetirement` 使用预先签发的 `MEDIA_API_TOKEN` 请求 `MEDIA_GATEWAY_ADDR`，先验证 `/api/auth/me` 成功，再检查旧创建、列表、撤销路由均为404。不会签发或撤销借用凭据，不模拟登录。

```bash
go -C media-cli test ./e2e -run '^TestLegacyTokenCRUDRetirement$' -count=1 -v
```

`service_token_retirement_test.go` 使用隔离PG：历史夹具显式迁移至16，再验证0017不改变历史、八种写操作均被拒绝、旧Platform未转移时整批回滚、down拒绝自动恢复旧认证，以及独立Platform和User停用流程不再写旧表。历史Platform迁移测试也固定schema16；一般夹具仍执行全部最新迁移。不得为构造历史夹具关闭运行环境的归档保护。

```bash
go -C media_auth test -tags=e2e -race ./e2e -run '^(TestRetiredServiceTokenArchive|TestPlatformToken(Migration|Readiness|Kratos))' -count=1 -v
```

运行环境 mTLS 门禁使用预启动 Auth；先完成[维护切换](../../media_auth/script/platform-token-operations.md)，再执行：

```bash
MEDIA_PLATFORM_TOKEN_ADDR=https://<Auth-private-IP>:8890 \
MEDIA_AUTH_PRIVATE_ADDR=http://<Auth-private-IP>:8001 \
MEDIA_PLATFORM_TEST_MATERIAL_DIR=/absolute/path/to/media_auth/.secrets/platform-token \
MEDIA_PLATFORM_AUDIT_FILE=/absolute/path/to/media_logs/media-auth/platform-audit/events.jsonl \
  go -C media-cli test ./e2e -run '^TestPlatformTokenMTLSRuntime$' -count=1 -v
```

该测试需要读取私有审计文件的授权身份；普通用户不可读时，应构建测试二进制并以授权身份执行，不能放宽 0600 权限。证书验证使用 `media_auth` ServerName，不关闭 TLS 校验。测试创建独立随机密码 User，获取/轮换/内省和真实异步文件落盘，不触碰原测试账号凭据，不启动服务或采集器。缺配置明确 Skip，文件不可读明确失败，不冒充已通过。

专用监听器的本地接线测试位于`media_auth/platform_runtime_test.go`：隔离CA/临时证书、真实Hertz HTTPS连接和生成路由，检查认证context/no-store/安全错误、普通HTTP伪造Header拒绝以及公共路由排除。用例使用测试桩，无运行数据库或旧凭据操作；这是接线门禁，不代替下述真实存储/用例测试或部署后的HTTP/CLI门禁。`biz/shared/platformmtls`另覆盖真实TLS握手证书边界及安全文件读取。

`platform_lifecycle_test.go`使用隔离PG验证完整历史的startup只读门禁，以及实际Kratos映射/密文确保和用户撤销的事务回滚、重试确认、审计原子性与恢复不复活。共享fixture直接执行SQL文件，不创建golang-migrate版本表；readiness测试先验证缺失拒绝，再显式创建版本/dirty夹具。不会修改运行数据或模拟真实企微/浏览器登录，不代替运行部署门禁。

```bash
go -C media_auth test -tags=e2e -race ./e2e -run '^TestPlatformToken(Readiness|Kratos)' -count=1
```

```bash
go -C media_auth test -race . ./biz/shared/platformmtls ./biz/middleware
```

`media_auth/e2e/platform_token_repo_test.go` 使用隔离PostgreSQL及仅测试用内存密钥，覆盖每用户当前凭据的并发唯一性、归属隔离、过期后显式撤销再替换、事务回滚、密文列约束及非空历史降级保护。不会读取或改写运行环境的旧凭据，不部署密钥，不模拟登录，也不启动业务服务：

```bash
go -C media_auth test -tags=e2e -race ./e2e -run '^TestPlatformToken' -count=1 -v
```

该存储门禁不能代替后续旧明文迁移、启动接线、内部调用者/purpose、撤销以及HTTP/CLI验收；加密与防调换单测位于`biz/shared/platformcipher`。

`platform_token_migration_test.go`另覆盖105条跨页/撤销历史、默认演练回滚、并行提交与重跑、目标复用、普通ServiceToken不变，以及坏Hash/空ID/缺owner/历史冲突/不可解密目标/取消后整批回滚。独立构建并执行`tools/platform-token-migrate`，验证默认不提交、显式apply和固定错误。只对隔离数据库操作，测试密钥/config为临时0600文件，捕获输出不打印；不迁移运行环境数据、不启动或切换业务服务。

```bash
go -C media_auth test -tags=e2e -race ./e2e -run '^TestPlatformTokenMigration' -count=1 -v
```

`platform_token_service_test.go`验证独立运行用例的同用户并发确保、过期替换、轮换/撤销历史、当前User停用/删除、跨User隔离、审计写入失败回滚、不可解密当前禁止隐式替换，以及读取后并发轮换的晚到内省拒绝。0016同时检查固定审计列、lease/attempt约束和非空历史down保护；不输出Secret。CallerVerifier使用显式测试桩，不能冒充mTLS或真实HTTP边界验收；此阶段不部署密钥、不切换认证或迁移运行数据，也不证明Outbox已投递。

```bash
go -C media_auth test -tags=e2e -race ./e2e -run '^TestPlatformTokenCore' -count=1 -v
```

`platform_audit_delivery_test.go`验证真实PostgreSQL的SKIP LOCKED、lease到期/新旧CAS、终态、attempts饱和及数据库实际时钟防止旧时间确认；并用临时0600 JSONL文件验证事务外写入+Sync、确认失败后关闭/重开sink与投递器重建、稳定event ID重投，以及单项失败不阻塞其他归属。读取只检查固定元数据，不打印完整日志；不会读取/解密凭据或启动观测后端，不替代运行环境启动接线和真实内部认证验收。文件sink允许重复event ID，读取需跳过空行、隔离损坏行并去重。

```bash
go -C media_auth test -tags=e2e -race ./e2e -run '^TestPlatformAudit' -count=1 -v
# 凭据存储/用例/迁移/投递一并回归
go -C media_auth test -tags=e2e -race ./e2e -run '^TestPlatform(Audit|Token)' -count=1 -v
```

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

业务授权门禁`TestPersonalDispatchAuthorizationIsolation`使用隔离PG及明确引擎桩，逐项改变User、企业、Actor、credential、instance、auth source、attempt，拒绝后无lease/status变化、Inbox写入或引擎更新。正确身份complete直接提交并核对原Task，重放不新增Inbox。它不创建运行环境身份，不代替公开网关JWT/撤销验证。

```bash
go -C media_workflow test -tags=e2e -race ./e2e \
  -run '^(TestPersonalDispatchAuthorizationIsolation|TestLegacyDispatchInboxConcurrentReplayAndDeliveryRetry)$' -count=1 -v
```

后一个门禁仅保留历史Inbox表/仓储的并发去重与旧协议兼容证据，投递复现在测试函数中，不代表生产仍有发送器，也不证明当前直接交付。当前恢复门禁见文首原生Task响应丢失测试与客户端真实子进程恢复；完整网关/CLI执行另验。

### Personal Worker 单样本执行门禁

真实 Personal Worker 单流程门禁需要先按 `media_worker/README.md` 启动独立客户端，再显式执行；测试本身不模拟 Worker，不启动服务，不输出凭据或签名资源 URL：

```bash
MEDIA_E2E_PERSONAL_WORKER_SAMPLE=1 \
MEDIA_E2E_SAMPLE_WORKFLOW_ID=<published-four-step-sample-id> \
go -C media-cli test ./e2e -run '^TestWorkflowPersonalWorkerFixture$' -count=1 -v
```

沿用上文网关/User API Token/Organization 环境变量。当前用户有未结束 Job 时拒绝提交。失败/超时保留执行供排查，不能自动重提；使用 `MEDIA_E2E_SAMPLE_JOB_ID=<existing-job-id>` 仅验收已有执行。样本完成后须用户确认，再迁移其他 Workflow 数据。

正常CLI提交是独立门禁，使用当前公开素材选择与对象解析，不调用旧Reachability。先dry-run，再核对队列/启动限定Capability的独立Worker后，只提交一条；移除dry-run后打印的Job ID用于上述已有Job观察模式，不再让测试创建另一条Job：

```bash
APP_ENV=local MEDIA_WORKFLOW_ADDR="$MEDIA_GATEWAY_ADDR" \
MEDIA_MATERIALS_ADDR="$MEDIA_GATEWAY_ADDR" \
go -C media-cli run . media_workflow submit-job --dry-run \
  --organization-id <organization_id> --workflow-id <published_workflow_id> \
  --product-id <product_id> --virtual-ip-id <virtual_ip_id> \
  --material 'movement=<material_id>,pose=<material_id>,scene=<material_id>'
```

`--asset 'input_name=<generated_asset_id>'` 选择schema声明的产物输入；不按类型自动挑记录。请求身份来自同一套`MEDIA_API_TOKEN`，不再支持手填`--user-id`。dry-run只输出输入计数/版本摘要，没有短时URL。该CLI门禁不替代HTTP执行/结果及审批门禁，也不作为前端旅程验收。

四节点 `TestWorkflowPersonalWorkerFixture` 可显式设置 `MEDIA_E2E_REGENERATE_STEP_KEY=model_image`，对该节点第一次审批调用重新生成，第二次审批再确认，并要求最终四节点成功。测试 Workflow 应把该节点配置为 `always`；其他环境变量及外部 fixture Worker 沿用单样本门禁。必须先审计用户待执行队列，不原位修改已发布 Compilation；测试结束停止 fixture Worker，不停止常驻后端。此门禁只证明实际单候选审批/重新生成链路，不替代多候选业务 API、并发决定、持久化 Outbox 和故障恢复测试。

该样本审批命令使用显式稳定幂等键并重复提交，要求返回同一 Review 回执。完成后可设置 `MEDIA_E2E_REVIEW_JOB_ID=<该样本 Job ID>`，运行 `go -C media-cli test ./e2e -run '^TestWorkflowReviewReceiptsGateway$' -count=1 -v`；它只重放已有决定，验证终态 202/applied 原回执与相反决定 409，不创建或重新执行任务。User Token 和网关配置沿用上述门禁，凭证禁止输出。

可设置 `MEDIA_E2E_REGENERATE_FEEDBACK=<测试反馈>` 验证反馈决定的同键重放，并检查下一轮 `rendered_prompt` 的纯文本反馈后缀。需要新发布且声明 `last_regeneration_v1` 的定义；旧定义不原位升级，非空反馈明确拒绝。该 HTTP 门禁仍使用外部 fixture Worker，不证明模型理解反馈的生成质量。

多候选门禁设置 `MEDIA_E2E_MULTIPLE_STEP_KEY=model_image` 和 `MEDIA_E2E_SELECT_CANDIDATE=1`；外部 Worker 使用私有 `MEDIA_WORKER_FIXTURE_MODEL_RESULT_URLS` 返回两个不同受控图片对象。每次审批必须确有两候选，完成后校验选中数组位置与最终 Generated Asset 对象路径；不复制同一图片伪造多个候选。新 Workflow 可用 when_multiple_candidates 检验预期数量与实际数量不一致的 HUMAN 分支。测试结果仍明确为 mock，不能当作模型生成质量验收。

失败重试 HTTP 门禁另设置 `MEDIA_E2E_RETRY_STEP_KEY=model_image`，独立 Worker 显式配置 `MEDIA_WORKER_FIXTURE_FAIL_STEP=model_image`、`MEDIA_WORKER_FIXTURE_FAIL_CALL=2`，配合上述重新生成门禁。只有观察到 Job FAILED 且指定 StepRun FAILED，才发出一次用户 Retry；第二次失败直接报错，不自动重提 Job。最终该 StepRun 的 retry_count 必须为 1，反馈仍存在，四节点全部成功。失败夹具不代替网络响应丢失、持久化重试意图或并行恢复门禁；此测试也不证明独立 CLI 重试成功链路。

独立 CLI 门禁使用同一套网关/User Token 配置，`APP_ENV=local MEDIA_WORKFLOW_ADDR="$MEDIA_GATEWAY_ADDR" MEDIA_MATERIALS_ADDR="$MEDIA_GATEWAY_ADDR"`：

```bash
go -C media-cli run . media_workflow job get --id <job_id>
go -C media-cli run . media_workflow step-run get --id <step_run_id>
go -C media-cli run . media_workflow step-run confirm --id <step_run_id> --selected-index 1 --idempotency-key <stable_accept_key>
go -C media-cli run . media_workflow step-run regenerate --id <step_run_id> --feedback '<feedback>' --idempotency-key <stable_regenerate_key>
```

确认与重新生成是互斥决定，不能用上例对同轮都提交；验收已有完成样本时复用已保存的原键/原内容，只重放对应 Review，不执行新一轮。重复确认返回同一回执，同键相反决定必须失败。CLI 默认不打印 URL 或 prompt，测试凭据不得出现在命令参数或输出中。

### 已完成 Workflow 输出只读验收

独立 CLI 失败重试完整门禁为 `TestWorkflowRetryCLI`。先在 media-cli 根构建二进制，再审计队列、启动独立 fixture Worker（失败节点 model_image、失败序号 1，多候选为两个受控图片）。沿用网关、User API Token、企业、Workspace 和已发布四节点 Workflow 的环境配置：

```bash
go -C media-cli build -o /tmp/media-agent-cli-workflow-retry-test .
MEDIA_E2E_RETRY_CLI_SAMPLE=1 \
MEDIA_E2E_CLI_BINARY=/tmp/media-agent-cli-workflow-retry-test \
  go -C media-cli test ./e2e -run '^TestWorkflowRetryCLI$' -count=1 -v -timeout=3m
```

门禁不启动 Worker、不模拟 Worker、不模拟登录；通过独立 CLI 进程执行 dry-run、唯一 Submit、查询、失败节点 Retry、候选 Confirm，最后要求四节点产物均存在且模型节点 retry_count=1。测试设置 CLI 工作目录为 media-cli 根以加载配置，捕获但不打印失败输出。失败保留 Job，不能重复执行门禁造成另一条 Job；结束后停止测试 Worker，再用只读产物门禁验收。该测试不是重试意图持久化、并行恢复或真实模型质量证明。

普通启动的协议门禁位于 `media_workflow/e2e/start_idempotency_test.go`，只创建唯一命名 HUMAN 技术流程，结束时终止本次实例并删除本次定义，不消费用户队列：

Workflow 注册协议门禁：`MEDIA_E2E_CONDUCTOR_URL=http://127.0.0.1:8080 go -C media_workflow test -tags=e2e -race ./e2e -run '^TestConductorRegistrationFrozenDefinition$' -count=1 -v -timeout=90s`。仅创建唯一技术 HUMAN 定义，用代理丢弃实际 PUT 成功响应；要求读回精确版本恢复、同内容重放不写、不同内容拒绝且原定义不变。结束只删测试定义，不启动实例/Worker、不修改共享 Task Definition 或运行用户 Compilation。它不证明 Publication 本地事务、后台重试或公开 HTTP/CLI 发布接线。

原生初次启动仓储门禁为 `go -C media_workflow test -tags=e2e -race ./e2e -run '^TestExecutionStartOutboxPersistence$' -count=1 -v -timeout=2m`。它仅在 testcontainers PostgreSQL 应用迁移（含 017/018），验证 Job/StepRun/启动记录同事务、唯一性、lease fencing、发送前标记、不可盲重发的不确定状态、回执绑定原子性、未发送取消不再领取、重复取消、uncertain 晚到绑定不复活、cancel_pending 原实例清理与审计降级保护。不调用 Conductor、不创建运行用户 Job、不部署服务；不代替公开 Submit、后台投递或实际进程重启验收。

产物投影持久层门禁：`go -C media_workflow test -tags=e2e -race ./e2e -run '^TestExecutionProjectionAtomicPersistence$' -count=1 -v -timeout=2m`。隔离 PostgreSQL 使用真实 Service/Repository/Tx，状态实际更新后注入错误，要求产物引用与成功状态共同回滚；下一次查询恢复，终态读取不重复 Upsert；模拟素材调用期间取消后不得复活 Job。Engine 快照和 Materials 返回是控制端口，该门禁不证明真实 Conductor/Materials 网络恢复、公开 HTTP、CLI 或运行部署。

同一门禁还覆盖 Invoke 状态写入后失败时，候选清空被一起回滚；以及 Engine 旧快照读取后取消，Render/Invoke/Finalize/Approval 不再修改已取消聚合。不能据此宣称跨 Retry 轮次的过期快照已被 epoch 隔离。

该门禁现亦在引擎读取旧快照后，实际持久化 RetryHistory/RetryCount，再检查 Render/Invoke/Finalize/Approval/Job status 投影被拒绝、查询返回当前 running 状态且不调用素材端口。这只证明本地已经增加次数后的边界；引擎 rerun 与本地接受之间的窗口、素材端按生成轮次去重和真实网络恢复仍是独立门禁。

公开提交回执持久层门禁为 `go -C media_workflow test -tags=e2e -race ./e2e -run '^TestSubmissionAcceptancePersistence$' -count=1 -v -timeout=2m`。隔离 PostgreSQL 应用迁移 019，八路并发同键只接受同一完整聚合；检查回滚、不同摘要冲突、终态原回执、三层作用域隔离、事务要求、回执归属与降级审计保护。不启动 Conductor、不改运行用户数据；不代替服务端归一化/授权重放、HTTP/CLI 或部署验收。

公开 HTTP 提交门禁：配置网关/User API Token、`MEDIA_E2E_SUBMIT_SOURCE_JOB_ID`（已有可读完整输入样本）与 `MEDIA_E2E_SUBMIT_IDEMPOTENCY_SAMPLE=1`，运行 `go -C media-cli test ./e2e -run '^TestWorkflowSubmitIdempotencyGateway$' -count=1 -v -timeout=2m`。八请求共享一次新意图，必须返回同一个 Job；等待后台启动、取消，仅修改媒体签名后重放原 cancelled 状态，改请求版本返回409。仅取消已确认属于本门禁的 Job，不启动模型 Worker，不把取消样本当作四节点完成证明。

独立 CLI 门禁：先构建 CLI，配置绝对路径 `MEDIA_E2E_CLI_BINARY`、网关/User Token、企业/Workspace/已发布四节点 fixture Workflow，以及 `MEDIA_E2E_SUBMIT_CLI_SAMPLE=1`；运行 `go -C media-cli test ./e2e -run '^TestWorkflowSubmitIdempotencyCLI$' -count=1 -v -timeout=2m`。三个独立进程使用同键提交：首次、运行期间重放、取消后重放返回原 Job；只创建一个取消专用样本，不启动 Worker。命令失败不自动重发，私有 stdout/stderr 不输出；资源组装读取与 HTTP 并发门禁不是同一项证明。

启动投递/取消恢复的真实引擎门禁为 `MEDIA_E2E_CONDUCTOR_URL=http://127.0.0.1:8080 go -C media_workflow test -tags=e2e -race ./e2e -run '^TestExecutionStartDeliveryCancellationRecovery$' -count=1 -v -timeout=3m`。使用真实 Conductor、隔离 PostgreSQL 与唯一 HUMAN 技术定义，仅在代理注入已提交响应丢失/即时查询不可用；覆盖未发送取消、未知启动只查询恢复、未知启动后取消、终止响应丢失后只读终态收口。每条已发送样本只有一个 Start，取消样本只有一个 Terminate；定义与实例仅清理测试创建的目标。不调用真实模型，不等同公开 HTTP/CLI 单流程或独立进程崩溃验收。

独立进程恢复门禁：`MEDIA_E2E_CONDUCTOR_URL=http://127.0.0.1:8080 go -C media_workflow test -tags=e2e -race ./e2e -run '^TestExecutionStartRealProcessRestart$' -count=1 -v -timeout=3m`。子进程在真实持久领取、真实 Start 提交、本地绑定前，或真实 Terminate 提交、清理落盘前直接退出；另一个进程只使用数据库/引擎事实恢复。只提前隔离夹具租约，不修改运行 Job 时间。已提交动作的重复调用会使恢复子进程退出 88，防止被临时错误路径吞掉。mark-before-send 另作为安全边界：恢复仍 uncertain/pending、无实例，不重发；通过该断言不证明能自动启动，更不替代兼容引擎的启动去重门禁或在线 API 容器重启。

```bash
MEDIA_E2E_CONDUCTOR_URL=http://127.0.0.1:8080 \
  go -C media_workflow test -tags=e2e ./e2e \
  -run '^TestConductorStart(CommittedResponseLossRecovery|IdempotencyContract)$' -count=1 -v -timeout=90s
```

响应丢失门禁使用真实上游，仅在反向代理丢弃成功响应，要求 correlation 恢复同一 ID 且只有一次 Start。幂等门禁要求同一 `RETURN_EXISTING` 键在活动/终态都返回原实例；当前 3.32.4 镜像该门禁失败，不能开启不确定 Start 的自动重发或宣称可靠启动已完成。此门禁不替代 Submit 的数据库 Outbox、用户 API 或实际进程重启验收。

审批内部故障恢复协议门禁位于 `media_workflow/e2e/review_outbox_test.go`，连接预启动 Conductor 和 testcontainers 隔离 PostgreSQL，不改运行用户数据：

```bash
MEDIA_E2E_CONDUCTOR_URL=http://127.0.0.1:8080 \
  go -C media_workflow test -tags=e2e ./e2e \
  -run '^TestReviewDeliveryReal(ProcessRestart|ConductorLostResponse)$' -count=1 -v -timeout=3m
```

`ProcessRestart` 使用两个独立测试子进程：领取后直接退出，以及真实提交成功、回执落盘前直接退出。恢复进程回收旧 lease；已提交场景禁止再次提交，只查询原 HUMAN。为避免等待两分钟，仅推进隔离夹具的 lease 到期时间。连接字符串通过子进程环境传入，不打印捕获的子进程输出。该门禁不替代用户 HTTP、后台服务装配或部署中 API 容器重启验收。

`TestWorkflowCompletedOutputsGateway` 读取指定的已有四节点成功 Job，检查当前用户归属、每个节点成功且有产物、产物 Object Path 属于测试企业，再以不携带 API Token/Cookie 的独立 HTTP 客户端请求短时存储 URL。四个输出必须分别返回 `206 bytes 0-0/<positive-size>`，前三个为图片、最后一个为视频；不跟随重定向，不打印签名 URL。不提交、恢复、审批 Job，不启动 Worker。该门禁不代替新的四节点执行或前端预览验收。

```bash
MEDIA_E2E_COMPLETED_JOB_ID=<existing-completed-four-node-job-id> \
go -C media-cli test ./e2e -run '^TestWorkflowCompletedOutputsGateway$' -count=1 -v
```

环境变量沿用网关/User API Token/Organization 配置。`TestWorkflowCompletedOutputsProbeContract` 是独立本地桩测试，验证只读请求数量、企业查询参数与存储请求不携带凭据；不能代替实际媒体可读门禁。

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

`TestRequestCorrelationLogsGateway` 在同样只读请求基础上，额外读取可读的实际挂载应用日志，逐个核对响应trace ID、非零且不同于上游的server span及未采样标志00。显式要求当前服务关闭观测导出；缺日志配置为Skip，日志不可读或5秒内缺记录为失败。只扫描每个文件末尾最多2MiB，不打印完整日志/Secret。此门禁不证明异步执行关联，也不将两个请求携带同一个父trace解释为两服务间真实内部调用。

```bash
go -C media-cli test ./e2e -run '^TestRequestCorrelationGateway$' -count=1 -v

MEDIA_E2E_REQUEST_LOG_DIR=/absolute/path/to/media_agent/media_logs \
go -C media-cli test ./e2e -run '^TestRequestCorrelationLogsGateway$' -count=1 -v
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

### Agent 委托调用落盘审计

`TestAgentProxyAuditGateway`额外配置本地挂载日志根目录，读取真实Gateway、Workflow、Materials的JSON应用日志，验证同一trace_id下的User/Actor/credential/企业/Workspace与业务状态（包括拒绝响应），且审计不包含Secret或query。只申请/撤销测试Token、读取业务资源，不启动观测后端或生成任务：

```bash
MEDIA_E2E_AGENT_AUDIT_LOG_DIR=/absolute/path/to/media_agent/media_logs \
go -C media-cli test ./e2e -run '^TestAgentProxyAuditGateway$' -count=1 -v
```

沿用User API Token、网关与Workspace配置；日志目录必须可读。CLI另以临时Agent Token执行`agent-client request`，再按凭据管理ID在两侧审计日志核对trace_id；不得输出Token、响应正文或完整日志。审计是请求日志，不代替可靠业务事件或不可篡改账本。

### Agent 凭据撤销后台兜底

`media_auth/e2e/agent_access_revocation_test.go` 使用隔离 PostgreSQL，覆盖候选状态/Actor过滤、精确owner/企业边界、权威故障不写撤销意图、晚到签发及重启。配置现有 Talos 地址后，还创建唯一测试 Agent Key，验证持久意图经重建的撤销流程收敛。Control 身份事实由测试桩控制，不能用此测试证明可靠生命周期事件或真实 Control 状态变更；不修改现有 Workspace/Agent/Token、不启动 Runtime。

```bash
MEDIA_E2E_TALOS_URL=<existing-private-talos-address> \
go -C media_auth test -tags=e2e -race ./e2e -run '^TestAgentAccessRevocation' -count=1 -v
```

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

### Materials 生成结果登记持久化

在 `media_materials/` 内执行：

```bash
GOWORK=off go test -tags=e2e ./e2e -run 'TestGeneration(ResultReceipt|RegistrationService)Persistence' -race -count=1 -v
```

两项门禁分别覆盖 Repository 键锁/八路并发/共享事务回滚/不可变约束/软删除与降级保护，以及实际 Service + Repository 的同轮去重、新轮独立 ID、新 Service 实例重放和来源校验。测试只创建并清理自己的 PostgreSQL 容器，不读写业务库。来源和对象存储是受控端口，不证明运行服务的 HTTP、真实 Storage 或 Workflow/Conductor 联调；接口级验证仍需独立 HTTP 与 CLI 门禁。

补充执行：

```bash
GOWORK=off go test -tags=e2e ./e2e -run '^TestGenerationRegistration(HTTPReplayPersistence|RealProcessRestart)$' -race -count=1
```

前者用Hertz `ut`覆盖实际IDL/路由/内部鉴权/handler与持久层，丢弃首个成功响应后重放；不是TCP断链测试。后者在真正子进程提交后直接退出，再由另一子进程恢复；重放时任何Asset保存或Storage访问均导致专用失败退出码，必须原ID且仍仅一份Asset/回执。禁止输出私有DSN、子进程原始错误或签名URL。仅测试持久化与进程恢复，不启动业务HTTP服务，也不替代网关与完整CLI门禁。

`TestGenerationRegistrationTCPLostResponse` 再通过受控TCP故障适配器，在真实handler提交后关闭socket；显式同键请求返回原回执且两表仍各1行。该适配器将网络请求转给Hertz `ut`，不等同于部署业务服务或网关的断网测试。Workflow `biz/shared/client` 的注册协议单元测试单独验证真实客户端不自动重发、ResultKey/内部Token透传与错误不泄露，不能用其模拟提供者冒充Materials持久化证明。

### Workflow 冻结编译持久化

发布恢复：在 `media_workflow/` 内执行 `MEDIA_E2E_CONDUCTOR_URL=http://127.0.0.1:8080 go test -tags=e2e ./e2e -run '^TestPublicationDelivery(RealConductorRecovery|RealProcessRestart)$' -race -count=1`。使用自己的隔离数据库与唯一技术TaskDefinition/Workflow metadata，不启动实例/Worker。分别覆盖写后代理503、本地完成回滚后仅查询恢复，以及真实子进程注册后退出、另一子进程回收claim恢复（恢复代理禁止所有非GET并检查0重写）。测试结束只清理自己的技术元数据。DSN仅经私有子进程环境传递，禁止输出子进程原始错误；不替代公开publish API/CLI、权限或标准main后台装配验收。

审批周期门禁在 `media_workflow/` 内运行 `go test -tags=e2e ./e2e -run '^(TestReviewRetryEpochPersistence|TestExecutionProjectionAtomicPersistence)$' -race -count=1`，覆盖真实接受事务锁内周期校验、原键重放不重绑定、同数字generation跨Retry的旧Review隔离、epoch持久化/不可变与降级保护。引擎快照受控，AppendRetryHistory仅代表已提交本地边界，不证明实际rerun窗口恢复。新审批由标准服务冻结epoch，旧null记录不回填。

`MEDIA_E2E_CONDUCTOR_URL=http://127.0.0.1:8080 go test -tags=e2e ./e2e -run '^(TestReviewDeliveryRealConductorLostResponse|TestReviewDeliveryRealProcessRestart)$' -race -count=1` 另验证冻结HUMAN Task ID的真实提交、丢响应后精确查询与进程恢复。禁止以Task Reference重新查找后提交新任务，不向客户端开放此内部接口。

在 `media_workflow/` 内运行 `go test -tags=e2e ./e2e -run '^TestCompilationFrozenPersistence$' -race -count=1`。真实隔离PostgreSQL验证8路同内容并发只插入1条、调用方事务回滚、JSON键顺序/空白重放、不同IR/Definition/hash/引擎身份冲突、不重置生命周期、首次注册时间不变与晚到失败不回退；migration020另限制直接SQL修改冻结内容/成功状态/注册时间。注册失败可对原冻结内容重新注册成功，新Workflow版本可保存独立编译记录。测试不调用Conductor，不改业务库、不启动Worker，不替代Publication Outbox或用户发布HTTP/CLI验收。

`MEDIA_E2E_CONDUCTOR_URL=http://127.0.0.1:8080 go test -tags=e2e ./e2e -run '^TestConductorTaskDefinitionFrozenRegistration$' -race -count=1` 使用独立技术TaskDefinition名称并在结束后清理，代理将实际POST成功响应替换成503，验证GET原定义恢复、重放0额外写入、不同契约拒绝且不PUT。无业务实例/Worker/模型执行或用户任务元数据改写。

设置 `MEDIA_E2E_EXISTING_TASK_DEFINITIONS=1` 并运行 `TestConductorPublishedTaskDefinitionsCompatibility`，对当前四节点样本的6个TaskDefinition作只读兼容预检；测试代理禁止任何非GET请求，存在冲突时不能修复线上定义。该门禁只适用于文档样本契约，不是其他用户Capability的通用政策。
