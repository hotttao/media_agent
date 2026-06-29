// tracing.go：服务端 OpenTelemetry 链路追踪。
//
// 实现（对齐官方文档 cloudwego.io Hertz OpenTelemetry）：
//   - 进程级 provider 由 ServerSuite.InitObservability 委托 hertz_infra/globalhertz
//     （obs-opentelemetry/provider.NewOpenTelemetryProvider）初始化；
//   - server tracer 通过 hertztracing.NewServerTracer() 返回 serverconfig.Option，
//     经 buildTracerOptions 拼入 server options；
//   - 中间件层 hertztracing.ServerMiddleware(cfg) 接管 span 生命周期。
package serverhertz

import (
	"github.com/cloudwego/hertz/pkg/app"
	hertzconfig "github.com/cloudwego/hertz/pkg/common/config"
	hertztracing "github.com/hertz-contrib/obs-opentelemetry/tracing"

	configpb "media_agent/hertz_gen/config"
)

// newTracingMiddleware 返回 server-side tracing middleware。
// otel 未启用时返回 nil（buildMiddlewareChain 自动跳过）。
func newTracingMiddleware(cfg *configpb.Config) app.HandlerFunc {
	otel := cfg.GetOtel()
	if otel == nil || !otel.GetEnabled() {
		return nil
	}
	_, tracerCfg := hertztracing.NewServerTracer()
	return hertztracing.ServerMiddleware(tracerCfg)
}

// buildTracerOptions 把 OTel server tracer（+ Prometheus metrics tracer）拼成 server options。
//
//   - otel.enabled：append hertztracing.NewServerTracer() 返回的 serverconfig.Option；
//   - prometheus.enabled：append server.WithTracer(prometheus.NewServerTracer(addr, path))，
//     monitor-prometheus 自管理独立 :9091 暴露 /metrics（pull 模式）。
//
// 两者未启用均不挂载，返回 nil。
func buildTracerOptions(cfg *configpb.Config) []hertzconfig.Option {
	if cfg == nil {
		return nil
	}
	var opts []hertzconfig.Option
	if otel := cfg.GetOtel(); otel != nil && otel.GetEnabled() {
		tracer, _ := hertztracing.NewServerTracer()
		opts = append(opts, tracer)
	}
	if p := cfg.GetPrometheus(); p != nil && p.GetEnabled() {
		opts = append(opts, newPrometheusServerTracer(p))
	}
	return opts
}
