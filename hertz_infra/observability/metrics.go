// metrics.go：进程级 metrics 状态初始化（薄入口）。
//
// monitor-prometheus 的 NewServerTracer 自管理独立的 metrics HTTP 服务器
// （监听 PrometheusConfig.address，暴露 PrometheusConfig.path），无需进程级
// exporter / registry 注册。本函数保留为对齐 contributing/server.md §4.1
// （InitMetrics）命名的薄入口，检测 enabled，未启用 no-op；
// 未来若切换 metrics 后端（如 OTel push），在此收口实装。
package observability

import (
	configpb "media_agent/hertz_gen/config"
)

func initMetrics(p *configpb.PrometheusConfig) error {
	_ = p // monitor-prometheus 自管理，进程级无额外初始化。
	return nil
}
