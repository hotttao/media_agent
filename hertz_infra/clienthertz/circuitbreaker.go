// circuitbreaker.go：客户端熔断 / 降级。
//
// 走 hertz-contrib/opensergo（sentinel adaptor）。
package clienthertz

import (
	hclient "github.com/cloudwego/hertz/pkg/app/client"

	configpb "media_agent/hertz_gen/config"
)

func applyCircuitBreakerMiddleware(cli *hclient.Client, cb *configpb.CircuitBreakerConfig, downstream string) {
	if cli == nil || cb == nil || !cb.GetEnabled() {
		return
	}
	_ = downstream
	// TODO(infra): cli.Use(adapter.SentinelClientMiddleware(adapter.WithClientResourceExtractor(...)))
}
