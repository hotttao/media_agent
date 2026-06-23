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
