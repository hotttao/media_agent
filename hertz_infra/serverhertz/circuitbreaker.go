// circuitbreaker.go：服务端熔断 / 降级。
//
// 走 hertz-contrib/opensergo（sentinel adaptor）。占位实现，规则下发由 sentinel-go SDK 处理。
package serverhertz

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"

	configpb "media_agent/hertz_gen/config"
)

func newCircuitBreakerMiddleware(cfg *configpb.CircuitBreakerConfig) app.HandlerFunc {
	if cfg == nil || !cfg.GetEnabled() {
		return nil
	}
	// TODO(infra): 接入 sentinel circuit breaker 规则下发 + adapter.SentinelServerMiddleware()。
	return func(ctx context.Context, c *app.RequestContext) {
		_ = ctx
		_ = c
	}
}
