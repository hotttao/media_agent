// discovery.go：客户端服务发现。
//
// 使用 hertz-contrib/registry/consul 的 NewConsulResolver + sd.Discovery 中间件。
//
// 当前依赖未引入时为 no-op；引入后真实实现：
//
//	cc, _ := consulapi.NewClient(...)
//	r := consul.NewConsulResolver(cc)
//	cli.Use(sd.Discovery(r))
package clienthertz

import (
	hclient "github.com/cloudwego/hertz/pkg/app/client"

	configpb "media_agent/hertz_gen/config"
)

func applyDiscovery(cli *hclient.Client, cd *configpb.ConsulDiscoveryConfig, lb string) {
	if cli == nil || cd == nil || !cd.GetEnabled() {
		return
	}
	_ = lb
	// TODO(infra): 接入 hertz-contrib/registry/consul.NewConsulResolver + sd.Discovery。
}
