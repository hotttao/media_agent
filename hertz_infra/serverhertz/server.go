// server.go：基于 ServerSuite 构造 Hertz server 实例 + 挂载全局中间件。
package serverhertz

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/config"

	configpb "media_agent/hertz_gen/config"
)

// NewServer 基于 suite 构造 Hertz server，并按约定顺序挂载全局 middleware。
//
// 中间件顺序见 middleware.go：recovery → requestid → tracing → metrics → accesslog → cors → ratelimit → circuitbreaker。
//
// 启动期一次性消费 suite.Config()（转发 loader.Current()）构造 middleware chain
// 与 pprof / health 路由——这些启动后不可变；可热更新能力由组件自己在
// NewServerSuite 内部 loader.Subscribe 订阅。
func NewServer(suite *ServerSuite) *server.Hertz {
	h := server.New(suite.Options()...)
	cfg := suite.Config()

	for _, mw := range buildMiddlewareChain(cfg) {
		h.Use(mw)
	}

	// pprof / health 走路由挂载，不走中间件链路
	mountPprof(h, cfg)
	mountHealth(h, cfg)

	return h
}

// buildServerOptions 从 cfg.Server 派生 Hertz server 启动 option。
//
// 仅处理纯启动参数（监听地址、读写超时）；注册中心走 buildRegistry，tracer 走 buildTracerOptions。
func buildServerOptions(cfg *configpb.Config) []config.Option {
	opts := []config.Option{}
	srv := cfg.GetServer()
	if srv != nil && srv.GetAddress() != "" {
		opts = append(opts, server.WithHostPorts(srv.GetAddress()))
	}
	if t := srv.GetTimeout(); t != nil && t.GetEnabled() {
		if t.GetReadTimeoutMs() > 0 {
			opts = append(opts, server.WithReadTimeout(msToDuration(t.GetReadTimeoutMs())))
		}
		if t.GetWriteTimeoutMs() > 0 {
			opts = append(opts, server.WithWriteTimeout(msToDuration(t.GetWriteTimeoutMs())))
		}
	}
	return opts
}
