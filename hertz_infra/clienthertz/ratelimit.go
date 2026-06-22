// ratelimit.go：客户端限流。
//
// 走 sentinel adapter 客户端版本。资源 key 推荐 "<caller_app>-><callee_app>"。
package clienthertz

import (
	hclient "github.com/cloudwego/hertz/pkg/app/client"

	configpb "media_agent/hertz_gen/config"
)

func applyRateLimitMiddleware(cli *hclient.Client, rl *configpb.RateLimitConfig, downstream string) {
	if cli == nil || rl == nil || !rl.GetEnabled() {
		return
	}
	_ = downstream
	// TODO(infra): cli.Use(adapter.SentinelClientMiddleware(adapter.WithClientResourceExtractor(...)))
}
