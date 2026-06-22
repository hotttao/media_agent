// ratelimit.go：服务端限流。
//
// 走 hertz-contrib/opensergo（sentinel adaptor），按资源粒度做 QPS 限流。
// 资源名规则：默认 "<app.name>:<method>:<path>"，由 sentinel 配置或运行期 LoadRules 控制。
package serverhertz

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"

	configpb "media_agent/hertz_gen/config"
)

// newRateLimitMiddleware 当 enabled=true 时返回 sentinel 服务端 middleware。
//
// 规则加载（sentinel.LoadRules）由 main 启动期完成，这里只负责装载 middleware；
// 详细规则下发可走 OpenSergo 控制面或本地 rule 文件，二者皆走 sentinel-go SDK。
func newRateLimitMiddleware(cfg *configpb.RateLimitConfig) app.HandlerFunc {
	if cfg == nil || !cfg.GetEnabled() {
		return nil
	}
	// TODO(infra): 接入 hertz-contrib/opensergo adaptor.SentinelServerMiddleware()。
	// 当前依赖未引入时返回 no-op，避免阻塞 build。
	return func(ctx context.Context, c *app.RequestContext) {
		// 真正的实现：
		//   sentinelOpts := []adapter.Option{adapter.WithServerResourceExtractor(extractor)}
		//   adapter.SentinelServerMiddleware(sentinelOpts...)(ctx, c)
		_ = ctx
		_ = c
	}
}
