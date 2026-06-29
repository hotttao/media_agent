# E2E 测试

借助实现的 [cli 工具](../../media-cli/)，完成 e2e 测试。

## 治理能力 e2e

启动被测 server（带治理能力 + 测试端点）：

```bash
cd media_example && go run ../media-cli serve --conf conf/local.yaml
# 或指定配置：go run ../media-cli serve --conf <path>
```

逐项验证服务端 / 客户端治理能力：

```bash
media-cli infra requestid      # X-Request-ID 透传 / 生成
media-cli infra accesslog      # 触发 access log
media-cli infra recovery       # panic recovery
media-cli infra timeout        # 读写超时
media-cli infra ratelimit      # 限流
media-cli infra circuitbreaker # 熔断
media-cli infra cors           # CORS
media-cli infra health         # /health
media-cli infra pprof          # /debug/pprof
media-cli client ...           # 客户端治理（timeout/retry/circuitbreaker/ratelimit）
```

## 可观测性 e2e（Trace + Metrics + Log）

### 1. 启动观测后端栈

```bash
cd docker-compose
./prepare.sh                                                    # 创建 /data/<service> 目录
docker compose -f docker-compose.yml up -d                      # collector + jaeger + prometheus + loki + grafana + promtail
docker compose ps                                               # 确认全部 Up
```

栈拓扑（详见 `docs/plan/observability.md`）：

- OTel Trace：app ─OTLP gRPC─→ otel-collector ─→ jaeger
- Metrics：prometheus scrape `host:9091/metrics`（monitor-prometheus pull）
- Log：app 文件日志 `media_logs/<app>/app.log` ─promtail─→ loki
- 可视化：grafana（:3000，admin/admin，预置 Prometheus/Loki/Jaeger 数据源）

组件访问地址：

| 组件 | 地址 |
|------|------|
| Hertz 服务 | http://127.0.0.1:8888 |
| Prometheus | http://127.0.0.1:9090 |
| Jaeger UI | http://127.0.0.1:16686 |
| Grafana | http://127.0.0.1:3000 |
| Loki API | http://127.0.0.1:3100 |
| OTel Collector OTLP | 127.0.0.1:4317 |

### 2. 启动被测 server（开启观测三件套）

```bash
cd media_example
OTEL_ENABLED=true \
PROMETHEUS_ENABLED=true \
LOG_ENABLED=true \
LOG_DIR=/home/tao/code/github/media_agent/media_logs/media_example \
go run ../media-cli serve --conf conf/local.yaml
```

> `LOG_DIR` 必须为 `media_logs/<app>` 绝对路径，promtail 才能采集到。

### 3. 验证

```bash
media-cli infra metrics                      # ✅ /metrics 返回 prometheus 格式（:9091）
media-cli infra trace                        # ✅ Jaeger 查到 span
media-cli infra log                          # ✅ Loki 查到 /_test/echo 日志行
```

未启用对应能力或后端栈未起时，命令返回 `⚠️ skip`（非失败）；能力已开但断言不通过返回 `❌`。

### 4. Grafana 关联验证

打开 http://127.0.0.1:3000 → Explore：

- Prometheus：查 `hertz_server_throughput` / `hertz_server_latency_us`；
- Loki：按 `{job="media-app"}` 查日志，日志行的 TraceID 字段可跳转 Jaeger；
- Jaeger：按 service=`media-example` 查链路。

## 联调排障：按 trace_id 关联全链路日志

联调接口出错时，**以 trace_id 为线索，从 client 侧发起点开始，过滤出本次调用经过的所有服务日志**，定位 bug。前提：各服务（含 media-cli）开启 `OTEL_ENABLED=true` + `LOG_ENABLED=true`，日志落各自 `media_logs/<app>/app.log`，promtail 统一采集到 Loki 并把 `trace_id` 提升为 label。

### 数据流

```text
media-cli (client 起点_span) ──traceparent header──→ media_example (server_span) ──→ 下游服务 ...
   │ media_logs/media-cli/app.log            │ media_logs/media_example/app.log
   │ {trace_id, span_id}                     │ {trace_id, span_id}
   └────────── promtail ──→ Loki ──→ 按 trace_id 聚合 ──→ Grafana
```

- media-cli 每次发起下游调用前用 `withSpan(ctx, name)` 在 client 侧建 span，并 `hlog.CtxInfof(ctx, ...)` 记录——该条日志携带本次调用的 trace_id（client 起点）。
- `clienthertz` 的 `hertztracing.ClientMiddleware()` 把 trace_id 通过 `traceparent` header 透传给下游。
- 下游 `serverhertz` 的 `hertztracing.ServerMiddleware` 续接同一 trace，其 access log / 业务日志携带相同 trace_id。
- 所有服务日志经 promtail 采集到 Loki，`trace_id` 已提升为 label。

### 排障步骤

1. **触发一次出错调用**（例）：

   ```bash
   OTEL_ENABLED=true LOG_ENABLED=true \
     go run ./media-cli client timeout --host http://127.0.0.1:8888
   ```

   命令输出 / media-cli 日志会带本次调用的 `trace_id`（也可从 media_logs/media-cli/app.log 取最后一条）。

2. **从 media-cli 日志取 trace_id**：

   ```bash
   grep -o '"trace_id":"[^"]*"' media_logs/media-cli/app.log | tail -1
   ```

3. **在 Loki 按 trace_id 过滤全链路日志**（API 或 Grafana Explore）：

   ```bash
   curl -G http://127.0.0.1:3100/loki/api/v1/query \
     --data-urlencode 'query={trace_id="<TRACE_ID>"}'
   ```

   返回的 stream 覆盖 media-cli + media-example + 所有下游服务本次调用产生的日志行，按时间排序即还原完整调用链上下文。

4. **在 Jaeger 对照链路**：`http://127.0.0.1:16686` 按该 trace_id 查 span 拓扑，定位耗时/异常 span，再回到 Loki 看对应服务的详细日志。

### 规范要求

- **client 侧必须用 `withSpan` 建起点并 `hlog.CtxInfof(ctx, ...)` 记录**——否则 client 日志不带 trace_id，无法从 client 起关联。
- **业务日志必须用 `hlog.CtxXX(ctx, ...)`**（传 ctx），不传 ctx 不会注入 trace_id。
- **每个服务日志目录用 `app.name` 命名**（`media_logs/<app.name>/app.log`），promtail 按目录区分来源，避免混肴。
- 联调脚本应输出本次调用的 trace_id，便于人工或自动接续 Loki/Jaeger 查询。

---

## go test e2e（testcontainers，按需细粒度断言）

治理 / HTTP / 可观测 e2e 走 media-cli（脚本驱动，见上文）；但 **migration 正确性/幂等性、跨 domain 事务回滚、repository 行为**等需要细粒度断言的场景，media-cli 测不了，用 `go test` + [testcontainers-go](https://golang.testcontainers.org/) 起真实容器测。

### 分工

| 测试类型 | 驱动方式 | 覆盖 |
|---------|---------|------|
| 治理 / HTTP / 可观测 | media-cli 命令（脚本） | 限流 / 熔断 / tracing / 日志 / swagger |
| 代码层细粒度（migration / 事务 / repo） | `go test -tags=e2e`（testcontainers） | 迁移幂等 / 表存在 / 事务提交回滚 / 数据一致性 |

### 目录组织（可扩展结构）

每个服务的 e2e 测试收口在 `<app>/e2e/`，独立 `package e2e`，与业务代码物理隔离：

```text
<app>/e2e/
├── e2e_test.go            # //go:build e2e + TestMain（进程级共享资源）+ 包级 build tag
├── <topic>_test.go        # 按主题分文件：migration_test.go / like_flow_test.go / ...
└── fixtures/              # 资源 fixture + 测试替身
    ├── mysql.go           # NewMySQL / NewDatabase：容器 + 隔离 database，t.Cleanup teardown
    ├── migrate.go         # Migrate(t, db)：封装 dal.Migrate
    ├── app.go             # NewXxxEnv(t)：装配 service/workflow 供测试用
    ├── assert.go          # 复用断言辅助（TableExists / LikesCount ...）
    └── fakes.go           # 测试替身（FailingCounterRepo ...）
```

### 资源管理范式（setup / teardown）

**核心**：每个资源一个 `NewXxx(t)` fixture 函数，setup 资源并用 `t.Cleanup` 注册 teardown；测试只声明「需要什么资源」，不写 cleanup。`t.Cleanup` 按 LIFO 自动执行（测试结束/失败/panic 都触发）。

```go
// fixtures/mysql.go：测试调 NewDatabase(t)，不管容器怎么起、database 怎么删。
func NewDatabase(t *testing.T) *Database {
    // 在共享容器里建独立 database（隔离）
    root.Exec("CREATE DATABASE `e2e_N`")
    t.Cleanup(func() { root.Exec("DROP DATABASE `e2e_N`") })  // teardown: 删 database
    db, _ := gorm.Open(...)
    t.Cleanup(func() { sql,_ := db.DB(); sql.Close() })        // teardown: 关连接
    return &Database{Gorm: db, DSN: dsn}
}
```

**重资源共享**：MySQL 容器由 `TestMain`（`e2e_test.go`）起一次，全进程复用；各测试经 `NewDatabase` 派生独立 database 隔离，既不互相干扰又省容器启动开销。无 docker 时 `TestMain` skip 整个包。

```go
// e2e/e2e_test.go
func TestMain(m *testing.M) {
    mysql, err := fixtures.StartSharedMySQL(ctx)
    if err != nil { os.Exit(0) }            // 无 docker 不算失败
    defer mysql.Terminate(ctx)
    fixtures.SetSharedMySQL(mysql)
    os.Exit(m.Run())
}
```

### 新增 e2e 测试 / 新增资源（扩展指南）

**新增一个 e2e 测试**（如测订单流程）：
1. 在 `e2e/` 下新建 `<topic>_test.go`（如 `order_flow_test.go`），顶部 `//go:build e2e`；
2. 用现有 fixture 组合资源（如 `env := fixtures.NewOrderFlowEnv(t)`），写断言；
3. 不动 fixtures、不动 TestMain。

**新增一种资源**（如 Redis）：
1. 在 `e2e/fixtures/` 加 `<resource>.go`（如 `redis.go`），写 `NewRedis(t)` + `t.Cleanup` teardown；
2. 重资源需共享则在 `TestMain` 加 `StartSharedRedis` + `SetSharedRedis`，轻量则每测试独立起；
3. 在 `app.go` 加组合 fixture（如 `NewOrderFlowEnv` 引入 Redis）。

**新增测试替身**：放 `e2e/fixtures/fakes.go`，实现对应 domain 接口，供多个测试复用。

### 运行

```bash
# 需 docker（testcontainers 起容器）
cd <app> && go test -tags=e2e ./e2e/ -v
# 无 docker 自动 skip；普通 go test（无 -tags=e2e）不跑 e2e
```

### 约定

- **build tag**：`//go:build e2e` 只在 `e2e/` 包内声明（同包共享），普通 `go test` 不参与，避免拖慢。
- **隔离**：重资源（容器）TestMain 共享，数据资源（database/schema）每测试独立 + `t.Cleanup` 删，杜绝测试间状态泄漏。
- **migration 用独立 `*sql.DB`**：`dal.Migrate` 按 dsn 新开连接，迁移完即关，不碰业务 gorm 连接——golang-migrate 的 `m.Close()` 会关闭其驱动，复用 gorm 连接会导致业务连接被关。
- **迁移 SQL 运行时读磁盘**：migrations 不 embed，`dal.Migrate(dir, cfg)` 用 `os.DirFS(dir)` 读 `.sql`；e2e fixture `resolveMigrationsDir` 从测试 cwd 向上找 `migrations/` 解析绝对路径，不依赖 cwd。
- **失败注入用 fake**：回滚测试用 fake repo（实现 domain Repo 接口、返回 error），断言事务整体回滚，不依赖真实失败触发。
- **断言复用**：通用断言（查表/查计数）放 `fixtures/assert.go`，测试调 `fixtures.TableExists(t, ...)`，不重复写 SQL。
