# 结构重构方案：globalhertz + 多资源限流 + IDL e2e

> 状态：待实装
> 解决 docs/spec/problem.md 的 4 个问题。

## 1. 新建 `hertz_infra/globalhertz`，删除 `hertz_infra/observability`

进程级全局唯一状态收口到 `globalhertz`，serverhertz / clienthertz 只 import 它。

```
hertz_infra/globalhertz/
├── doc.go          # 包说明
├── otel.go         # otelProvider 全局单例 + InitOTel()（sync.Once 幂等）
├── logger.go       # hlog logger 全局替换 + InitLogger()（sync.Once 幂等）
├── sentinel.go     # sentinel.InitDefault() 全局单例 + InitSentinel()（sync.Once 幂等）
└── metrics.go      # (无全局状态；保留薄 InitMetrics 占位或删除)
```

- `observability/` 包**删除**，所有内容移入 `globalhertz`（重命名 import path）
- serverhertz/clienthertz 的 `InitObservability` → 委托 `globalhertz.Init*`
- `InitMetrics` 薄入口：monitor-prometheus 自管理，无全局状态，移到 serverhertz/metrics.go（client 不需要）

## 2. RateLimitConfig 改多资源（repeated）

proto：
```proto
message RateLimitConfig {
  bool enabled = 1;
  string resource = 2;           // 资源名，如 "media-example:GET:/hello"；空=该条不下发
  double threshold = 3;          // QPS 阈值
  int32 stat_interval_ms = 4;
}
// Config:
repeated RateLimitConfig rate_limit = 43;  // 多条限流规则
```

`serverhertz/ratelimit.go`：
- `loadFlowRules(rules []*configpb.RateLimitConfig) error`：遍历下发多条 `flow.Rule`
- `newRateLimitMiddleware`：middleware 仍按动态资源名提取（`<app>:<method>:<path>`），规则按 resource 匹配
- 热更新：`NewServerSuite` 内 `loader.Subscribe` → `loadFlowRules(newCfg.GetRateLimit())`

yaml：
```yaml
rate_limit:
  - enabled: true
    resource: "media-example:GET:/hello"
    threshold: 10
    stat_interval_ms: 1000
```

## 3. IDL test service + 生成 typed client（media-cli e2e）

新增 `idl/media_example/test.proto`：
```proto
service TestService {
  rpc Echo(TestEchoReq) returns (TestEchoResp) { option (api.get) = "/_test/echo"; }
  rpc Slow(TestSlowReq) returns (TestEchoResp) { option (api.get) = "/_test/slow"; }
  rpc Error(TestErrorReq) returns (TestEchoResp) { option (api.get) = "/_test/error"; }
  rpc Panic(...) ... { option (api.get) = "/_test/panic"; }
}
```

- `make hz-client`：生成 `hertz_gen/media_example/test_service.go` typed client
- `make hz-update`（media_example）：生成 handler 骨架 + 路由
- media_example/biz/handler/test：实现 4 个端点（echo/slow/error/panic）
- **media_example/main.go** 不再需要 serve 命令的 `mountTestEndpoints`——改由 media_example 自身注册生成的 test 路由
  - 但 media_example 是正式服务，不应含测试端点 → **取舍**：test handler 只在 media_example 注册，由 build tag 或直接保留（e2e 基础环境）。或：media-cli serve 仍用裸路由，但 media-cli 的 client e2e 用生成的 typed client 调 media_example 的 test 路由
  - **决定**：test service 放 media_example（生成 handler + 路由），media-cli serve 删除 mountTestEndpoints，统一由 media_example 提供端点。media-cli 只做 client。

## 4. media-cli client e2e 改用 typed client + clienthertz 治理

media-cli/cmd/client.go：
- 删除 `buildClient`（裸 hclient）
- 改用 `hertz_gen/media_example.NewTestServiceClient(host, options...)`：
  - options 注入 `clienthertz.ClientSuite` 的 `clientOption`（timeout/retry）+ `middlewares`（tracing/circuitbreaker）
- 通过生成的 typed client 调 echo/slow/error，验证治理行为（timeout/retry/circuitbreaker）

## 文件清单

**新增**
- `hertz_infra/globalhertz/{doc,otel,logger,sentinel}.go`
- `idl/media_example/test.proto`
- `hertz_gen/media_example/test_service.go`（make hz-client 生成）
- `media_example/biz/handler/test/test_service.go`（make hz-update 生成骨架 + 填充）
- `media_example/biz/router/test/{test.go,middleware.go}`（生成）

**删除**
- `hertz_infra/observability/`（全部）

**修改**
- `idl/config/config.proto`（rate_limit → repeated，RateLimitConfig 加 resource）
- `hertz_infra/serverhertz/{suite,tracing,metrics,ratelimit}.go`（import globalhertz；loadFlowRules 遍历）
- `hertz_infra/clienthertz/{suite,tracing,metrics,circuitbreaker}.go`（import globalhertz）
- `media-cli/cmd/{client,client_timeout,client_retry,client_circuitbreaker,hello}.go`（typed client）
- `media-cli/cmd/serve.go`（删除 mountTestEndpoints）
- `media_example/conf/*.yaml`（rate_limit 改列表）
- `harness/contributing/{server,client}.md`（目录结构更新）
