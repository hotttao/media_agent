// metrics.go：服务端 Prometheus 指标（pull 模式）。
//
// 实装（对齐官方文档 cloudwego.io Hertz Prometheus）：
//
//	h := server.Default(server.WithTracer(prometheus.NewServerTracer(":9091", "/metrics")))
//
// monitor-prometheus 的 NewServerTracer 启动独立 HTTP 服务器监听
// PrometheusConfig.address（默认 :9091），暴露 PrometheusConfig.path（默认 /metrics），
// 并按请求自动埋点（请求量 / 时延，tag=HTTP Method + statusCode）。
//
// 该 tracer 在 buildTracerOptions 中通过 server.WithTracer 注入，无需 middleware；
// newMetricsMiddleware 保持 nil（buildMiddlewareChain 自动跳过）。
// 进程级状态初始化归 ServerSuite.InitObservability。
package serverhertz

import (
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	hertzconfig "github.com/cloudwego/hertz/pkg/common/config"
	"github.com/hertz-contrib/monitor-prometheus"

	configpb "media_agent/hertz_gen/config"
)

// newMetricsMiddleware 默认 nil：metrics 走 monitor-prometheus server tracer
// （buildTracerOptions 注入），不需要单独中间件。保留接口以兼容未来自定义埋点场景。
func newMetricsMiddleware(cfg *configpb.PrometheusConfig) app.HandlerFunc {
	_ = cfg
	return nil
}

// initMetrics 进程级 metrics 状态初始化（薄 no-op）。
// monitor-prometheus 的 NewServerTracer 自管理独立 HTTP 服务器（buildTracerOptions 注入），
// 进程级无需额外 exporter / registry 注册。保留入口以便未来切换 metrics 后端时收口。
func initMetrics(p *configpb.PrometheusConfig) error {
	_ = p
	return nil
}

// newPrometheusServerTracer 构造 monitor-prometheus server tracer，包成 server option。
// address / path 缺失走默认（:9091 / /metrics）。
func newPrometheusServerTracer(p *configpb.PrometheusConfig) hertzconfig.Option {
	addr := p.GetAddress()
	if addr == "" {
		addr = ":9091"
	}
	path := p.GetPath()
	if path == "" {
		path = "/metrics"
	}
	return server.WithTracer(prometheus.NewServerTracer(addr, path))
}
