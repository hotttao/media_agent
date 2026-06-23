# 可观测性（Trace + Metrics + Log）落地计划

> 状态：待评审（已按 `server.md` / `client.md` 更新版 review 对齐）
> 范围：`hertz_infra` 实装 OTel Trace / Prometheus Metrics / Zap Log；`docker-compose/` 启动全套观测后端；能力默认继承到 `media_example` 与 `media-cli`；作为 e2e 测试基础环境。

## 0. 规范依据与口径

- **结构 / 配置规范**（裁决优先级）：
  - `harness/contributing/config.md`：配置 schema 规则（proto → `make pb-config`，`enabled` 门控，`app.name`）。
  - `harness/contributing/server.md`：服务端治理收口 `hertz_infra/serverhertz`，`ServerSuite` 装配面，main.go 启动顺序（§8），可观测性规则（§7）。
  - `harness/contributing/client.md`：客户端治理收口 `hertz_infra/clienthertz`，`ClientSuite` 装配面，进程级初始化（§6.2/§7.2）。
  - `harness/contributing/architecture.md`：层边界与 import 规则（冲突时优先）。
- **观测后端栈**：以 `docs/spec/opentelemetry.md` 拓扑（Zap→Filebeat→Loki / OTel Trace→Collector→Jaeger / Metrics `/metrics`→Prometheus / Grafana）为参考，**不照抄其代码**。
- **官方文档最佳实践**（实装严格对齐）：
  - OTel：https://www.cloudwego.io/zh/docs/hertz/tutorials/third-party/open-telemetry/
  - Prometheus：https://www.cloudwego.io/zh/docs/hertz/tutorials/third-party/prometheus/
  - Zap：https://www.cloudwego.io/zh/docs/hertz/tutorials/third-party/log/zap/
- 配置 schema 走 proto → `make pb-config`，业务只读 Go 结构体。

## 1. 目标拓扑

```
media_example / media-cli serve (host)
├─ Zap 结构化日志(带 trace_id/span_id) → media_logs/<app>/app.log ─ Filebeat ─→ Loki
├─ OTel Tracing(server+client middleware 自动 span) ─OTLP gRPC─→ OTel Collector ─→ Jaeger
└─ Prometheus Tracer(monitor-prometheus, 独立 :9091 暴露 /metrics) ← Prometheus scrape ─→ Prometheus
统一可视化：Grafana(Prometheus + Loki + Jaeger)
中转核心：OTel Collector 只收敛 trace；metrics 走 monitor-prometheus pull，不绕 collector，避免重复计数
```

口径决策（对齐官方文档 + 现有配置段 + server.md §4.1）：
- **Trace**：`obs-opentelemetry/provider.NewOpenTelemetryProvider`（仅 tracing，`WithEnableMetrics` 不开）+ `hertztracing.NewServerTracer()`/`ServerMiddleware`（server）+ `hertztracing.ClientMiddleware()`（client）。OTLP push → collector → jaeger。`sample_ratio` 归 `OtelConfig`。
- **Metrics**：`monitor-prometheus.NewServerTracer(addr, path)` 经 `server.WithTracer(...)` 注入，**独立监听** `PrometheusConfig.address`(:9091) 暴露 `/metrics`，Prometheus pull。这是官方 Prometheus 文档的 documented 方式，也与现有 `serverhertz/metrics.go` 占位注释一致。
  - 与 server.md §4.1 三件套（`InitMetrics()` + `RegisterMetricsRoute()` + `Middlewares()`）的协调：以 `monitor-prometheus` tracer 为实装核心（tracer 同时完成"每请求埋点 + 暴露 /metrics"），`InitMetrics` / `RegisterMetricsRoute` 提供为符合规范命名的薄入口（tracer 内部已自管理服务器与路由时为 no-op / 转发），`newMetricsMiddleware` 保持 nil。代价见 §8。
- **Log**：`hertz-contrib/logger/zap.NewLogger(WithCores(CoreConfig{Enc,Ws,Lvl}), WithZapOptions(...))` + lumberjack 落盘 + `obs-opentelemetry/logging/zap`（zapotel）桥接注入 trace_id/span_id。app 写 `media_logs/<app>/app.log`，Filebeat 采集推 Loki。
  - server.md §7："具体的日志 / 追踪 SDK 留在 `hertz_infra/observability`"——结构化 app 日志收口 `hertz_infra/observability`。
- 现有 `access_log`（AccessLogConfig）只管访问日志中间件格式，与结构化 app 日志职责不同，**保留分离**。

## 2. 配置 schema 变更（`idl/config/config.proto`）

新增 `LogConfig` 顶层段（可观测区，紧接 `access_log` 之后），按 config.md 规范（snake_case、首字段 `bool enabled = 1`）：

```proto
message LogConfig {
  bool enabled = 1;
  string level = 2;        // debug / info / warn / error，默认 info
  string dir = 3;          // 日志目录，如 /home/.../media_logs/media_example
  string filename = 4;     // 默认 app.log
  int32 max_size_mb = 5;   // lumberjack，默认 100
  int32 max_backups = 6;   // 默认 10
  int32 max_age_days = 7;  // 默认 7
  bool compress = 8;       // 默认 true
  bool console = 9;        // 同时写 stdout，默认 true（e2e 友好）
}
```

`Config` 顶层加：`LogConfig log = 33;`（接 `access_log = 32` 之后）。

`OtelConfig`（已有 `export_endpoint / insecure / sample_ratio`）与 `PrometheusConfig`（已有 `address / path`）**不改**。

流程：改 proto → `make pb-config` 重新生成 `hertz_gen/config/config.pb.go`（禁止手改）→ 更新 `conf/*.yaml` 模板 → 业务读取。

## 3. `hertz_infra` 代码实装

### 3.1 新增 `hertz_infra/observability` 包（进程级观测初始化唯一收口）

为什么独立包（对齐 server.md §2 目录结构列出的 `hertz_infra/observability/`）：`config` 包按规范不得依赖 OTel/zap SDK；进程级 TracerProvider / zap logger 被 server 与 client 共享，放独立包避免 `clienthertz → serverhertz` 反向依赖，保持 serverhertz / clienthertz 对称收口。

server.md §8 把 `InitTracing` / `InitMetrics` 挂在 `serverhertz` 入口、client.md §6.2 把同名列挂在 `clienthertz`——二者都是薄入口，真正构造逻辑收在 `observability`：

文件：
- `hertz_infra/observability/otel.go`
  - `InitOTel(app *configpb.AppConfig, otel *configpb.OtelConfig) (shutdown func(context.Context) error)`
  - `enabled=false` / `nil` → 返回 no-op shutdown，不报错（`enabled` 语义）。
  - 对齐官方 OTel 文档：`provider.NewOpenTelemetryProvider` + `WithServiceName(app.name)` + `WithExportEndpoint(otel.export_endpoint)` + `WithInsecure()` + `WithEnableTracing(true)` + `WithSampler(trace.ParentBased(trace.TraceIDRatioBased(otel.sample_ratio)))` + `WithDeploymentEnvironment(app.env)` + `WithResourceAttributes(attr.String("service.version", app.version))`。
  - 不调 `WithEnableMetrics` —— metrics 走 monitor-prometheus pull，避免双计数。
  - 内部 `sync.Once` 幂等：server 与 client 都可能触发，只构造一次全局 provider。
- `hertz_infra/observability/logger.go`
  - `InitLogger(app *configpb.AppConfig, log *configpb.LogConfig) error`
  - 对齐官方 zap 文档：lumberjack `WriteSyncer` + `zapcore.NewMultiWriteSyncer(file, stdout?)` + `zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())` + `zap.AtomicLevel`（level 字段映射）。
  - `hertzzap.NewLogger(hertzzap.WithCores(hertzzap.CoreConfig{Enc, Ws, Lvl}), hertzzap.WithZapOptions(zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel)))`。
  - 再用 `zapotel.NewLogger(zapotel.WithLogger(hZap), zapotel.WithTraceIDField(true), zapotel.WithSpanIDField(true), zapotel.WithRecordStackTraceInSpan(true))` 包装，`hlog.SetLogger(traceLogger)`。
  - `enabled=false` → 不替换 hlog，保持 hertz 默认。
- `hertz_infra/observability/metrics.go`
  - `InitMetrics(p *configpb.PrometheusConfig) error`：进程级 prometheus 状态（monitor-prometheus 自管理时为薄/no-op，保留入口对齐规范）。
- `hertz_infra/observability/doc.go`：包说明 + 调用顺序（先 `InitLogger` 再 `InitOTel`；main defer shutdown）。

### 3.2 `serverhertz` 实装（对齐 server.md §8 启动顺序与 §4.1 接入命名）

- **`ServerSuite` 扩展**（§5.1 / §8）：
  - 新增 `Tracing()` / `Metrics()` getter：分别返回 `cfg.GetOtel()` / `cfg.GetPrometheus()`，供 `InitTracing` / `InitMetrics` 入参（§8 `suite.Tracing` / `suite.Metrics`）。
  - 新增 `Middlewares()`：导出 `buildMiddlewareChain(cfg)`，对齐 §5.1/§12.1（`ServerSuite.Middlewares()` 顺序稳定）。`NewServer` 改为调用 `suite.Middlewares()`。
- **`tracing.go`**（占位→实装）：
  - `newTracingMiddleware(cfg)`：`otel.enabled` 时 `tracer, cfg := hertztracing.NewServerTracer()` 返回 `hertztracing.ServerMiddleware(cfg)`；否则 nil。
  - `buildTracerOptions(cfg)`：`otel.enabled` 时 append `hertztracing.NewServerTracer()`（server tracer，`server.Option`）；`prometheus.enabled` 时 append `prometheus.NewServerTracer(PrometheusConfig.address, PrometheusConfig.path)`（metrics tracer，独立 :9091）。
  - 新增 `InitTracing(ctx context.Context, otel *configpb.OtelConfig) func(context.Context) error`：委托 `observability.InitOTel(cfg.GetApp(), otel)`，返回 shutdown closure。
- **`metrics.go`**（占位→实装）：
  - `newMetricsMiddleware(cfg)`：保持 nil（metrics 走 `server.WithTracer(prometheus.NewServerTracer(addr, path))`，在 `buildTracerOptions` 注入，无需 middleware）。保留注释说明。
  - `InitMetrics(p *configpb.PrometheusConfig) error`：委托 `observability.InitMetrics(p)`。
  - `RegisterMetricsRoute(h *server.Hertz, cfg *configpb.Config)`：monitor-prometheus tracer 自管理服务器与路由，本函数为规范命名薄入口（检测 `prometheus.enabled`，未启用 no-op）。
- **运维端点导出化**（§8 第 10 步 `RegisterHealthRoutes` / `RegisterPProfRoutes` / `RegisterMetricsRoute`）：
  - `mountHealth` → `RegisterHealthRoutes(h, cfg)`；`mountPprof` → `RegisterPProfRoutes(h, cfg)`；新增 `RegisterMetricsRoute(h, cfg)`。
  - `NewServer` 不再内部挂载 health/pprof/metrics route，改由 main 显式调用（对齐 §8 第 10 步）。
- 中间件顺序不变（`middleware.go`：recovery → requestid → tracing → metrics → accesslog → ratelimit → circuitbreaker → cors）。

### 3.3 `clienthertz` 实装（对齐 client.md §6.2）

- **`tracing.go`**：`applyTracingMiddleware` → `cli.Use(hertztracing.ClientMiddleware())`（依赖 `observability` 初始化的全局 TracerProvider）。新增 `InitTracing(app, otel)` 薄入口委托 `observability.InitOTel`（client.md §6.2 "初始化 OTel provider（进程级）"）。
- **`metrics.go`**：官方文档 client 无 tracer 接口。`applyMetricsMiddleware` 保持 no-op + 注释（client metrics 暂走 tracing 侧 span events，避免引入未经验证埋点）。`InitMetrics(p)` 薄入口委托 `observability.InitMetrics`。
- `suite.go` `Middlewares()` 顺序不变（tracing → metrics → ratelimit → circuitbreaker）。

### 3.4 依赖新增（`hertz_infra/go.mod`）

```
github.com/hertz-contrib/obs-opentelemetry          # provider + tracing + logging/zap
github.com/hertz-contrib/monitor-prometheus         # NewServerTracer (pull /metrics)
github.com/hertz-contrib/logger/zap                 # hertzzap
go.uber.org/zap
github.com/natefinch/lumberjack
go.opentelemetry.io/otel
go.opentelemetry.io/otel/sdk
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc
```
（`media-cli/go.mod` 间接继承，`go mod tidy`。）

## 4. `docker-compose/` 观测后端栈

目录布局（配置文件放 `docker-compose/`，数据/日志放 `/data/<service>/`，app 日志放 `media_logs/<app>/`）：

```
docker-compose/
├── docker-compose.yml           # 根目录只保留编排入口
├── prepare.sh
├── otel-collector/config.yaml
├── jaeger/README.md             # all-in-one 无配置文件，环境变量驱动
├── prometheus/config.yml
├── loki/config.yaml
├── grafana/provisioning/{datasources,dashboards}/...
└── promtail/config.yaml
/data/                    # 绝对路径，每个 infra 服务一个目录（基础服务的数据和日志各自落自己目录）
├── otel-collector/
├── jaeger/
├── prometheus/
├── loki/
└── grafana/
media_logs/               # 微服务 app 日志，每个服务一个目录（已存在 media_example/）
└── media_example/app.log
```

服务清单（`docker-compose.yml`）：

| 服务 | 镜像 | 端口 | 数据卷 | 说明 |
|------|------|------|--------|------|
| otel-collector | `otel/opentelemetry-collector-contrib` | 4317(otlp grpc), 4318(otlp http), 8888(自身 metrics) | `/data/otel-collector` | 接收 OTLP trace，export → jaeger(otlp) |
| jaeger | `jaegertracing/all-in-one` | 16686(UI), 4317(OTLP 接收) | `/data/jaeger`(badger 存储) | collector 用 OTLP exporter 推 `jaeger:4317` |
| prometheus | `prom/prometheus` | 9090 | `/data/prometheus` | scrape app `:9091/metrics`（host-gateway）+ collector `:8888` |
| loki | `grafana/loki` | 3100 | `/data/loki` | 接收 promtail 推送 |
| grafana | `grafana/grafana` | 3000 | `/data/grafana` | 预置 3 数据源（Prometheus / Loki / Jaeger） |
| promtail | `grafana/promtail` | — | 挂 `../media_logs` 只读 | 采集 `media_logs/*/*.log` → loki |

关键配置点：
- Linux 下 prometheus / promtail 访问 host 上的 app：prometheus 用 `extra_hosts: ["host.docker.internal:host-gateway"]`；promtail 直接挂载 host `../media_logs` 只读。prometheus scrape `host.docker.internal:9091`（media_example / media-cli serve 的 :9091 metrics 端口）。
- `otel-collector/config.yaml`：receivers=otlp(grpc+http)；processors=batch；exporters=otlp/jaeger(`jaeger:4317`, insecure) + logging(debug)；pipeline traces。
- `prometheus/config.yml`：scrape `host.docker.internal:9091`（app `/metrics`）+ `otel-collector:8888`（collector 自身指标）。
- `promtail/config.yaml`：filestream 读 `/logs/*/*.log`（挂 `../media_logs`），json stage 解析 zap 行，trace_id 提升为 label，push → `loki:3100`。
  - **注**：spec `opentelemetry.md` 用 filebeat→Loki，但 filebeat 8.x 已移除 `output.loki`；改用 Loki 官方 agent promtail，拓扑不变（app 文件日志 → 采集 agent → Loki）。
- grafana provisioning：datasources 自动配 Prometheus/Loki/Jaeger，免手工。
- `prepare.sh` 启动前创建所有 `/data/<service>` 目录并设置权限。

## 5. 默认继承到 `media_example` 与 `media-cli`

### 5.1 `media_example/main.go`（对齐 server.md §8 启动顺序）

在现有顺序中插入观测初始化 + 显式 Register 路由：

```go
cfg := loader.Current()
serverhertz.InitLogger(cfg.GetLog())                              // 先 logger（薄入口委托 observability）
shutdownTracing := serverhertz.InitTracing(ctx, suite.Tracing())  // 进程级 tracing provider
defer shutdownTracing(ctx)
serverhertz.InitMetrics(suite.Metrics())                          // 进程级 metrics
// ... NewServer(suite) ...
register(h)
serverhertz.RegisterHealthRoutes(h, cfg)
serverhertz.RegisterPProfRoutes(h, cfg)
serverhertz.RegisterMetricsRoute(h, cfg)
// ... RegisterService / defer Deregister ...
h.Spin()
```

> 注：server.md §8 的 `RegisterService(ctx, suite)` / `RegistryHandle` 形态与当前 `suite.RegisterService()`（返回 error、内部持 registrar）不同；registry 本任务未启用（consul.enabled=false），保持现有方法形态，不重构 handle 模型（超范围）。
> graceful shutdown（`<-ctx.Done()`）保持现有 `h.Spin()` 行为，不引入额外 ctx 编排（超范围）。

`conf/local.yaml` + `.env` 补 `log:` 段与开关 env（`LOG_ENABLED` / `LOG_DIR` / `LOG_LEVEL`；`OTEL_ENABLED` / `PROMETHEUS_ENABLED` 默认仍 false，e2e 时显式打开）。

### 5.2 `media-cli/cmd/serve.go`

同 §5.1：`loadLoaderOrDie` 后插入 `InitLogger` / `InitTracing` + defer shutdown / `InitMetrics`，并在 `mountTestEndpoints` 后调用 `RegisterHealthRoutes` / `RegisterPProfRoutes` / `RegisterMetricsRoute`，使 `media-cli serve` 具备完整观测能力（e2e 基础环境）。

### 5.3 e2e 验证命令（`media-cli/cmd/`）

新增（与现有 `infra_*` 风格一致）：
- `infra_trace`：发请求 → 调 Jaeger API(`http://localhost:16686/api/traces?service=<app.name>`) 断言出现 span。
- `infra_log`：发请求 → 调 Loki API(`http://localhost:3100/loki/api/v1/query`) 按 trace_id/关键字断言日志入库。
- 现有 `infra_metrics` **调整**：metrics 现由 `monitor-prometheus` 独立监听 `PrometheusConfig.address`(:9091)，不在主服务端口。把测试目标从 `FlagHost+/metrics` 改为可配置 metrics 端点（新增 `--metrics-url` 默认 `http://127.0.0.1:9091/metrics`）。断言逻辑（`# HELP`/`# TYPE` 格式）不变。

## 6. 文件清单

**新增**
- `hertz_infra/observability/{doc,otel,logger,metrics}.go`
- `docker-compose/{docker-compose.yml,prepare.sh,otel-collector/config.yaml,jaeger/README.md,prometheus/config.yml,loki/config.yaml,promtail/config.yaml,grafana/provisioning/*}`
- `media-cli/cmd/infra_trace.go`、`media-cli/cmd/infra_log.go`
- `docs/plan/observability.md`（本文件）

**修改**
- `idl/config/config.proto`（加 `LogConfig` + `log` 字段）
- `hertz_infra/serverhertz/suite.go`（加 `Tracing()`/`Metrics()`/`Middlewares()`）
- `hertz_infra/serverhertz/{server,tracing,metrics,health,pprof}.go`（占位→实装 + 导出 Register*）
- `hertz_infra/clienthertz/{tracing,metrics}.go`（TODO→实装 + `Init*` 薄入口）
- `hertz_infra/go.mod`（依赖）
- `media_example/main.go`、`media_example/conf/local.yaml`、`media_example/.env`
- `media-cli/cmd/serve.go`、`media-cli/cmd/infra_metrics.go`、`media-cli/go.mod`
- `harness/contributing/e2e.md`
- `hertz_gen/config/config.pb.go`（`make pb-config` 重新生成，禁止手改）

## 7. 验证

1. `make pb-config` 生成通过。
2. `gofmt -l .` 无输出；`go build ./...`（hertz_infra / media_example / media-cli）通过。
3. `make arch-check` 通过（`observability` 包只 import configpb + SDK，不被 domain/policy/service import；serverhertz/clienthertz 单向依赖 observability）。
4. `bash docker-compose/prepare.sh && docker compose -f docker-compose/docker-compose.yml up -d` 全套起活。
5. `OTEL_ENABLED=true LOG_ENABLED=true PROMETHEUS_ENABLED=true media-cli serve`。
6. `media-cli infra metrics` / `infra trace` / `infra log` 全绿。
7. Grafana(`:3000`) 三数据源可查到 trace / log / metric。

## 8. 风险与取舍

- **metrics 实装核心用 `monitor-prometheus` pull（:9091）**：对齐官方 Prometheus 文档 + 现有代码注释；server.md §4.1 列的 `InitMetrics`/`RegisterMetricsRoute`/`Middlewares` 三件套以薄入口对齐命名（tracer 自管理服务器与路由时为 no-op/转发）。若评审要求严格走 OTel metrics push（`obs-opentelemetry/metrics` + collector metric pipeline），则改 provider `WithEnableMetrics(true)` 并去 :9091，但会与官方 Prometheus 文档分叉、且需重写 `infra_metrics` 目标——本计划暂取 pull。
- `/data` 为绝对路径，需 host 可写；`prepare.sh` 负责建目录 + chown，e2e 文档标注前提。
- `obs-opentelemetry/logging/zap`（zapotel）桥接 API 以 obs-opentelemetry 仓库实际导出为准；若包路径/option 名有差异，实装时按仓库 README 校正（不影响整体结构）。
- promtail 采集文件日志而非 OTLP 推日志：对齐 spec 拓扑（Zap→采集 agent→Loki）；filebeat 8.x 已无 `output.loki`，故用 Loki 官方 promtail。
- registry / graceful-shutdown 的 `RegistryHandle` / `ctx` 形态未按 server.md §8/§11 重构（本任务未启用 consul，超范围）；保持现有方法形态。
