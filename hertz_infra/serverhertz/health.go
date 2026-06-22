// health.go：健康检查端点。
//
// 暴露 GET <path>，返回 200 + {"status":"ok"}。
package serverhertz

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/utils"

	configpb "media_agent/hertz_gen/config"
)

func mountHealth(h *server.Hertz, cfg *configpb.Config) {
	hc := cfg.GetHealth()
	if hc == nil || !hc.GetEnabled() {
		return
	}
	path := hc.GetPath()
	if path == "" {
		path = "/health"
	}
	h.GET(path, func(ctx context.Context, c *app.RequestContext) {
		c.JSON(200, utils.H{"status": "ok"})
	})
}
