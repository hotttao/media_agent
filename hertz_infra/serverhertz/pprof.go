// pprof.go：debug 端点。
//
// 走 hertz-contrib/pprof.Register(h, prefix)。
package serverhertz

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/hertz-contrib/pprof"

	configpb "media_agent/hertz_gen/config"
)

func mountPprof(h *server.Hertz, cfg *configpb.Config) {
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
