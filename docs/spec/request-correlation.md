# 默认关闭可观测性时的请求关联

## 范围

Hertz 服务及受治理客户端在 `otel.enabled=false` 时仍维护 W3C Trace Context，支持结构化日志关联；不启动 Collector、Jaeger、Loki、Grafana，不创建 exporter，不采样或记录 Span。开启 OTel 时继续使用既有 provider 和 Hertz 官方 tracing 插件。

## 实现契约

- `hertz_infra/tracecontext` 使用独立、NeverSample、无 exporter/processor 的本地 Tracer；不得替换全局 TracerProvider。
- 服务端从请求 `traceparent`/`tracestate` 提取父上下文，新建本地 server span。合法上游 trace ID 保持不变；缺失或非法值生成新 trace ID。服务端 span ID 必须不同于上游，响应 `X-Trace-ID` 对应处理请求的 context。
- `X-Request-ID` 与 W3C trace ID 是不同概念，不将任意 request ID 当作 trace ID。
- `hertz_infra/clienthertz` 默认新建本地 client span，注入 W3C 字段，清除复用 Request 的旧 trace 字段，不改 Authorization 或业务字段；透传取消与下游错误。
- `globalhertz.InitOTel` 在导出关闭时安装 W3C propagator，不启动全局 provider，供现有裸 HTTP Adapter 传递上下文。
- 请求日志必须使用 `hlog.Ctx*` 并传入请求 context；无 context 的启动/后台日志不伪造请求关联。请求 trace ID 不是认证或授权依据，不传递 baggage/Secret。

## 验收

单测覆盖合法/缺失/非法/全零父上下文、sampled 父上下文仍不采样、本地 client/server span ID 区分、响应 ID、旧 header 清理、凭据不变和错误透传。联调在默认关闭观测后端的实际服务中验证响应及落盘日志含同一 trace ID。尚未纳入跨异步 Job/Outbox/Conductor 的业务执行关联；该生命周期不能仅靠一次 HTTP 请求的 context 延长。
