// requestid.go：请求级 trace id（X-Request-ID）。
//
// 走 hertz-contrib/requestid，自动写入 response header；缺省生成 UUID。
package serverhertz

import (
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/hertz-contrib/requestid"

	configpb "media_agent/hertz_gen/config"
)

func newRequestIDMiddleware(cfg *configpb.RequestIdConfig) app.HandlerFunc {
	if cfg == nil || !cfg.GetEnabled() {
		return nil
	}
	opts := []requestid.Option{}
	if h := cfg.GetHeader(); h != "" {
		opts = append(opts, requestid.WithCustomHeaderStrKey(requestid.HeaderStrKey(h)))
	}
	return requestid.New(opts...)
}
