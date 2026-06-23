// metrics_route.go：/metrics 路由注册（薄 no-op，由 ServerSuite.RegisterRoutes 调用）。
//
// monitor-prometheus 的 NewServerTracer 自管理独立 HTTP 服务器（监听
// PrometheusConfig.address）并暴露 PrometheusConfig.path，/metrics 不在 Hertz
// server 上。本函数为对齐统一 RegisterRoutes 入口的薄实现：未启用 / 启用均无需
// 在 Hertz server 注册路由，保留入口以便未来切换 metrics 后端时收口。
package serverhertz

import (
	"github.com/cloudwego/hertz/pkg/app/server"

	configpb "media_agent/hertz_gen/config"
)

// registerMetricsRoute 注册 metrics 路由（no-op）。
//
// 当前为 no-op：metrics 由 monitor-prometheus server tracer 自管理暴露，
// 不在 Hertz server 注册。保留入口以便未来切换 metrics 后端时收口。
func registerMetricsRoute(h *server.Hertz, cfg *configpb.Config) {
	_ = h
	_ = cfg
}
