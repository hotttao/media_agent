// metrics.go：服务端 Prometheus 指标。
//
// 实现：buildTracerOptions 中通过 server.WithTracer(prometheus.NewServerTracer(addr, path))
// 注入；本文件提供请求级 metric middleware 占位（默认走 server tracer 已足够，无需 middleware）。
package serverhertz

import (
	"github.com/cloudwego/hertz/pkg/app"

	configpb "media_agent/hertz_gen/config"
)

// newMetricsMiddleware 默认 nil：metrics 走 server tracer，不需要单独中间件。
// 保留接口以兼容某些埋点场景（业务 label、自定义计数器）。
func newMetricsMiddleware(cfg *configpb.PrometheusConfig) app.HandlerFunc {
	if cfg == nil || !cfg.GetEnabled() {
		return nil
	}
	return nil
}
