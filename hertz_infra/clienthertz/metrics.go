// metrics.go：客户端指标。
//
// 走 server tracer 客户端版本（hertz-contrib/monitor-prometheus）；
// 当前简化为 no-op，业务侧暂不需要在 client middleware 显式埋点。
package clienthertz

import (
	hclient "github.com/cloudwego/hertz/pkg/app/client"

	configpb "media_agent/hertz_gen/config"
)

func applyMetricsMiddleware(cli *hclient.Client, p *configpb.PrometheusConfig) {
	if cli == nil || p == nil || !p.GetEnabled() {
		return
	}
	// TODO(infra): cli.Use(prometheus.ClientMiddleware()) 或自定义 hist counter。
}
