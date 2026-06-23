// server.go：Hertz server 启动 option 构造辅助。
//
// server 实例创建与 middleware 挂载归 ServerSuite.NewServer（见 suite.go）；
// 本文件只提供从 cfg.Server 派生纯启动参数的 buildServerOptions。
package serverhertz

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/config"

	configpb "media_agent/hertz_gen/config"
)

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
