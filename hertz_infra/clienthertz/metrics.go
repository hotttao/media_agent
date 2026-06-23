// metrics.go：客户端指标。
//
// 官方文档（cloudwego.io Hertz Prometheus）说明：client 暂无 Tracer 接口，
// 仅提供 middleware 能力。当前保留 no-op：client 侧调用 metrics 暂走 tracing
// 侧的 span events，避免引入未经验证的自定义埋点。
//
// 进程级状态初始化归 ClientSuite.InitObservability。
package clienthertz

import (
	hclient "github.com/cloudwego/hertz/pkg/app/client"

	configpb "media_agent/hertz_gen/config"
)

// applyMetricsMiddleware 当前 no-op：client 侧暂不单独埋点。
// 保留入口以便未来按需接入 prometheus client middleware。
func applyMetricsMiddleware(cli *hclient.Client, p *configpb.PrometheusConfig) {
	_ = cli
	_ = p
}
