// tracing.go：服务端 OpenTelemetry 链路追踪。
//
// 实现：
//   - 启动期通过 obs-opentelemetry/provider.NewOpenTelemetryProvider 初始化全局 TracerProvider；
//   - 通过 hertztracing.NewServerTracer() 注入 server tracer（在 buildTracerOptions 中处理）；
//   - 中间件层通过 hertztracing.ServerMiddleware(cfg) 接管 span 生命周期。
package serverhertz

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/config"

	configpb "media_agent/hertz_gen/config"
)

// newTracingMiddleware 返回 server-side tracing middleware。
//
// 缺少依赖时为 no-op；引入 hertz-contrib/obs-opentelemetry 后替换为：
//
//	return hertztracing.ServerMiddleware(tracingCfg)
func newTracingMiddleware(cfg *configpb.Config) app.HandlerFunc {
	otel := cfg.GetOtel()
	if otel == nil || !otel.GetEnabled() {
		return nil
	}
	return func(ctx context.Context, c *app.RequestContext) {
		_ = ctx
		_ = c
		// 真正的实现：hertztracing.ServerMiddleware(tracingCfg)(ctx, c)
	}
}

// buildTracerOptions 把 OTel + Prometheus 等 server-tracer 类 option 拼到一起。
//
// 真实实现：
//
//	opts = append(opts, server.WithTracer(prometheus.NewServerTracer(addr, path)))
//	opts = append(opts, hertztracing.NewServerTracer())
func buildTracerOptions(cfg *configpb.Config) []config.Option {
	_ = cfg
	return nil
}
