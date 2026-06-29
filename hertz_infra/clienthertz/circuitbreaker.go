// circuitbreaker.go：客户端熔断（熔断只在 client 侧，见 server.md §5.2）。
//
// 实装（对齐官方示例 hertz-examples/sentinel/hertz/client）：
//
//	c.Use(hertzSentinel.SentinelClientMiddleware(
//	    hertzSentinel.WithClientResourceExtractor(extractor),
//	    hertzSentinel.WithClientBlockFallback(fallback),
//	))
//
// 进程级 sentinel.InitDefault() 由 loadCircuitBreakerRules 懒初始化。
// 规则热更新：ForApp 内 loader.Subscribe → 刷新 atomic + loadCircuitBreakerRules（全量替换）。
// 资源名：默认 "<caller_app>-><callee_app>"，由 appName（callee）+ 进程 app.name（caller）拼。
package clienthertz

import (
	"context"
	"fmt"

	"github.com/alibaba/sentinel-golang/core/circuitbreaker"
	hclient "github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/protocol"
	hertzSentinel "github.com/hertz-contrib/opensergo/sentinel/adapter"

	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/globalhertz"
)

// circuitBreakerMiddleware 返回 sentinel 客户端熔断 middleware；cb.enabled=false / nil → nil。
// callerApp/calleeApp 用于资源名 "<caller>-><callee>"。
func circuitBreakerMiddleware(cb *configpb.CircuitBreakerConfig, callerApp, calleeApp string) hclient.Middleware {
	if cb == nil || !cb.GetEnabled() {
		return nil
	}
	opts := []hertzSentinel.ClientOption{
		hertzSentinel.WithClientResourceExtractor(func(ctx context.Context, req *protocol.Request, resp *protocol.Response) string {
			return callerApp + "->" + calleeApp
		}),
		hertzSentinel.WithClientBlockFallback(func(ctx context.Context, req *protocol.Request, resp *protocol.Response, blockErr error) error {
			return fmt.Errorf("circuit breaker open for %s->%s: %w", callerApp, calleeApp, blockErr)
		}),
	}
	return hertzSentinel.SentinelClientMiddleware(opts...)
}

// loadCircuitBreakerRules 把 CircuitBreakerConfig 转成 sentinel circuitbreaker.Rule 并下发。
// 懒初始化 sentinel（globalhertz.InitSentinel）；LoadRules 全量替换，支持热更新。
// 资源名固定 "<caller>-><callee>"，与 extractor 一致。
func loadCircuitBreakerRules(cb *configpb.CircuitBreakerConfig, callerApp, calleeApp string) error {
	if cb == nil || !cb.GetEnabled() {
		return nil
	}
	if err := globalhertz.InitSentinel(); err != nil {
		return err
	}
	strategy, ok := parseCBStrategy(cb.GetStrategy())
	if !ok {
		return fmt.Errorf("clienthertz: unknown circuit_breaker strategy %q", cb.GetStrategy())
	}
	rule := &circuitbreaker.Rule{
		Resource:         callerApp + "->" + calleeApp,
		Strategy:         strategy,
		Threshold:        cb.GetThreshold(),
		StatIntervalMs:   uint32(cb.GetStatIntervalMs()),
		RetryTimeoutMs:   uint32(cb.GetRetryTimeoutMs()),
		MinRequestAmount: uint64(cb.GetMinRequestAmount()),
		MaxAllowedRtMs:   uint64(cb.GetMaxAllowedRtMs()),
	}
	_, err := circuitbreaker.LoadRules([]*circuitbreaker.Rule{rule})
	return err
}

// parseCBStrategy 把配置字符串映射到 sentinel circuitbreaker.Strategy。
func parseCBStrategy(s string) (circuitbreaker.Strategy, bool) {
	switch s {
	case "slow_request_ratio", "SlowRequestRatio":
		return circuitbreaker.SlowRequestRatio, true
	case "error_ratio", "ErrorRatio":
		return circuitbreaker.ErrorRatio, true
	case "error_count", "ErrorCount":
		return circuitbreaker.ErrorCount, true
	default:
		return 0, false
	}
}
