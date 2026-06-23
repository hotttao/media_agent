// middleware.go：全局中间件顺序约束 + 工具函数。
//
// 顺序原则（外层 → 内层），对齐 server.md §5.2：
//
//	recovery   ─ 兜底 panic
//	requestid  ─ 注入 trace id
//	tracing    ─ otel span 起点
//	metrics    ─ prometheus 计数
//	accesslog  ─ 写访问日志（拿到 status / latency 后写）
//	ratelimit  ─ 限流（在业务前）
//	cors       ─ 处理预检（放最后，业务前一步）
//
// 治理职责（见 docs/plan/observability.md）：限流只在 server 侧，熔断只在 client 侧。
// 故 server 中间件链无 circuitbreaker；熔断归 clienthertz。
//
// 各 middleware 自身负责 enabled 判断；这里只决定顺序。
package serverhertz

import (
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	configpb "media_agent/hertz_gen/config"
)

func buildMiddlewareChain(cfg *configpb.Config) []app.HandlerFunc {
	chain := []app.HandlerFunc{}
	appName := ""
	if a := cfg.GetApp(); a != nil {
		appName = a.GetName()
	}
	for _, mw := range []app.HandlerFunc{
		newRecoveryMiddleware(cfg.GetRecovery()),
		newRequestIDMiddleware(cfg.GetRequestId()),
		newTracingMiddleware(cfg),
		newMetricsMiddleware(cfg.GetPrometheus()),
		newAccessLogMiddleware(cfg.GetAccessLog()),
		newRateLimitMiddleware(cfg.GetRateLimit(), appName), // rate_limit 现为 repeated
		newCORSMiddleware(cfg.GetCors()),
	} {
		if mw != nil {
			chain = append(chain, mw)
		}
	}
	return chain
}

func msToDuration(ms int32) time.Duration {
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

func secondsToDuration(s int32) time.Duration {
	if s <= 0 {
		return 0
	}
	return time.Duration(s) * time.Second
}
