// ratelimit.go：服务端限流（限流只在 server 侧，见 server.md §5.2）。
//
// 实装（对齐官方示例 hertz-examples/sentinel/hertz/server）：
//
//	h.Use(hertzSentinel.SentinelServerMiddleware(
//	    hertzSentinel.WithServerResourceExtractor(extractor),
//	    hertzSentinel.WithServerBlockFallback(fallback),
//	))
//
// 规则：Config.rate_limit 是 repeated，一个 server 可配多条规则，每条针对一个资源（resource）。
// middleware 按 "<app>:<method>:<path>" 动态提取资源名；loadFlowRules 把所有 enabled 规则
// 下发为 sentinel flow.Rule（flow.LoadRules 全量替换，支持热更新）。
// 进程级 sentinel.InitDefault() 由 loadFlowRules 懒初始化（globalhertz.InitSentinel）。
package serverhertz

import (
	"context"
	"log"

	"github.com/alibaba/sentinel-golang/core/flow"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	hertzSentinel "github.com/hertz-contrib/opensergo/sentinel/adapter"

	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/globalhertz"
)

// newRateLimitMiddleware 当存在 enabled 规则时返回 sentinel 服务端限流 middleware；否则 nil。
//
// appName 用于默认资源名 "<app>:<method>:<path>"。规则下发归 loadFlowRules（启动期 + 订阅热更新）。
// 即便规则列表为空，只要有一条 enabled，middleware 仍挂载（无匹配规则=放行）。
func newRateLimitMiddleware(rules []*configpb.RateLimitConfig, appName string) app.HandlerFunc {
	if !anyRateLimitEnabled(rules) {
		return nil
	}
	opts := []hertzSentinel.ServerOption{
		hertzSentinel.WithServerResourceExtractor(func(_ context.Context, c *app.RequestContext) string {
			return appName + ":" + string(c.Method()) + ":" + string(c.Request.Path())
		}),
		hertzSentinel.WithServerBlockFallback(func(ctx context.Context, c *app.RequestContext) {
			c.AbortWithStatusJSON(consts.StatusTooManyRequests, map[string]any{
				"err":  "rate limited",
				"code": 429,
			})
		}),
	}
	return hertzSentinel.SentinelServerMiddleware(opts...)
}

// anyRateLimitEnabled 返回 rules 中是否存在 enabled 规则。
func anyRateLimitEnabled(rules []*configpb.RateLimitConfig) bool {
	for _, r := range rules {
		if r != nil && r.GetEnabled() {
			return true
		}
	}
	return false
}

// loadFlowRules 把所有 enabled 且 resource 非空的规则转成 sentinel flow.Rule 并下发。
// 懒初始化 sentinel（globalhertz.InitSentinel）；flow.LoadRules 全量替换，支持热更新。
// resource 为空的规则不下发（资源名是动态的 <app>:<method>:<path>，需运行期按需建规则）。
func loadFlowRules(rules []*configpb.RateLimitConfig) error {
	if !anyRateLimitEnabled(rules) {
		return nil
	}
	if err := globalhertz.InitSentinel(); err != nil {
		return err
	}
	flowRules := make([]*flow.Rule, 0, len(rules))
	for _, r := range rules {
		if r == nil || !r.GetEnabled() {
			continue
		}
		resource := r.GetResource()
		if resource == "" {
			continue // 动态资源名场景：不在启动期下发固定规则
		}
		strategy := parseTokenStrategy(r.GetTokenCalculateStrategy(), resource)
		behavior := parseControlBehavior(r.GetControlBehavior(), resource)
		rule := &flow.Rule{
			Resource:               resource,
			Threshold:              r.GetThreshold(),
			TokenCalculateStrategy: strategy,
			ControlBehavior:        behavior,
			StatIntervalInMs:       uint32(r.GetStatIntervalMs()),
			MaxQueueingTimeMs:      uint32(r.GetMaxQueueingTimeMs()),
		}
		// warmup 需 WarmUpPeriodSec>0 且 WarmUpColdFactor>1，否则 flow.LoadRules 拒绝整批规则。
		if strategy == flow.WarmUp {
			if rule.WarmUpPeriodSec = uint32(r.GetWarmUpPeriodSec()); rule.WarmUpPeriodSec == 0 {
				rule.WarmUpPeriodSec = 10
			}
			if rule.WarmUpColdFactor = uint32(r.GetWarmUpColdFactor()); rule.WarmUpColdFactor <= 1 {
				rule.WarmUpColdFactor = 3
			}
		}
		flowRules = append(flowRules, rule)
	}
	if len(flowRules) == 0 {
		return nil
	}
	_, err := flow.LoadRules(flowRules)
	return err
}

// parseTokenStrategy 把配置字符串映射到 sentinel flow.TokenCalculateStrategy。
// memory_adaptive 缺内存阈值字段（proto 未暴露）无法正确配置，命中时告警并回退 direct。
// 空/未知串默认 direct（保持历史行为）。
func parseTokenStrategy(s, resource string) flow.TokenCalculateStrategy {
	switch s {
	case "", "direct":
		return flow.Direct
	case "warmup":
		return flow.WarmUp
	case "memory_adaptive":
		log.Printf("serverhertz: rate_limit resource %q token_calculate_strategy=memory_adaptive not fully configurable, fallback to direct", resource)
		return flow.Direct
	default:
		log.Printf("serverhertz: rate_limit resource %q unknown token_calculate_strategy %q, fallback to direct", resource, s)
		return flow.Direct
	}
}

// parseControlBehavior 把配置字符串映射到 sentinel flow.ControlBehavior。
// 空/未知串默认 reject（保持历史行为）。
func parseControlBehavior(s, resource string) flow.ControlBehavior {
	switch s {
	case "", "reject":
		return flow.Reject
	case "throttling":
		return flow.Throttling
	default:
		log.Printf("serverhertz: rate_limit resource %q unknown control_behavior %q, fallback to reject", resource, s)
		return flow.Reject
	}
}
