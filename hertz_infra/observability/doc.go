// Package observability 是跨服务共享的进程级可观测性初始化唯一收口。
//
// 职责（对齐 contributing/server.md §2 / §7 与 contributing/client.md §6.2）：
//   - 结构化应用日志：zap + lumberjack 落盘，obs-opentelemetry/logging/zap 桥接
//     自动从 context 注入 trace_id / span_id；
//   - OTel 链路追踪 provider：obs-opentelemetry/provider（仅 tracing，metrics 走
//     monitor-prometheus pull，不在 provider 侧开启，避免双计数）；
//   - Prometheus metrics 进程级状态（monitor-prometheus 自管理服务器与路由时为薄入口）。
//
// 为什么独立包：config 包按规范不得依赖 OTel/zap SDK；进程级 TracerProvider / logger
// 被 serverhertz 与 clienthertz 共享，放本包避免 clienthertz → serverhertz 反向依赖，
// 保持 serverhertz / clienthertz 对称收口、单向依赖 observability。
//
// 调用顺序（main.go）：先 InitLogger 再 InitOTel，defer shutdown。
package observability

import (
	"context"

	configpb "media_agent/hertz_gen/config"
)

// InitLogger 初始化结构化应用日志并替换 hertz 默认 hlog。
//
// log.enabled=false / nil → 不替换 hlog，保持 hertz 默认，不报错（enabled 语义）。
// 幂等：重复调用以首次为准（避免热更新重复替换 logger）。
func InitLogger(app *configpb.AppConfig, log *configpb.LogConfig) error {
	return initLogger(app, log)
}

// InitOTel 初始化进程级 OTel tracing provider，返回 shutdown closure。
//
// otel.enabled=false / nil → 返回 no-op shutdown，不报错（enabled 语义）。
// 幂等：server 与 client 都可能触发，内部 sync.Once 只构造一次全局 provider；
// 返回的 shutdown closure 同样幂等，多次调用安全。
func InitOTel(app *configpb.AppConfig, otel *configpb.OtelConfig) func(context.Context) error {
	return initOTel(app, otel)
}

// InitMetrics 初始化进程级 metrics 状态。
//
// monitor-prometheus 的 NewServerTracer 自管理独立的 metrics HTTP 服务器与路由，
// 进程级无需额外 exporter / registry 注册，本函数为对齐规范命名的薄入口
// （检测 enabled，未启用 no-op）。保留入口以便未来切换 metrics 后端时收口。
func InitMetrics(p *configpb.PrometheusConfig) error {
	return initMetrics(p)
}
