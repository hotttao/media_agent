// cors.go：跨域。
//
// 走 hertz-contrib/cors。
package serverhertz

import (
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/hertz-contrib/cors"

	configpb "media_agent/hertz_gen/config"
)

func newCORSMiddleware(cfg *configpb.CorsConfig) app.HandlerFunc {
	if cfg == nil || !cfg.GetEnabled() {
		return nil
	}
	c := cors.Config{
		AllowOrigins:     cfg.GetAllowOrigins(),
		AllowMethods:     cfg.GetAllowMethods(),
		AllowHeaders:     cfg.GetAllowHeaders(),
		AllowCredentials: cfg.GetAllowCredentials(),
		MaxAge:           secondsToDuration(cfg.GetMaxAgeSeconds()),
	}
	return cors.New(c)
}
