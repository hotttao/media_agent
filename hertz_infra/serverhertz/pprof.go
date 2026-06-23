// pprof.go：debug 端点。
//
// 走 hertz-contrib/pprof.Register(h, prefix)。
package serverhertz

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/hertz-contrib/pprof"

	configpb "media_agent/hertz_gen/config"
)

// registerPProfRoutes 注册 pprof debug 路由（由 ServerSuite.RegisterRoutes 调用）。
// pprof.enabled=false / 缺失时不注册（enabled 语义）。
func registerPProfRoutes(h *server.Hertz, cfg *configpb.Config) {
	p := cfg.GetPprof()
	if p == nil || !p.GetEnabled() {
		return
	}
	prefix := p.GetPrefix()
	if prefix == "" {
		prefix = "/debug/pprof"
	}
	pprof.Register(h, prefix)
}
