// registry.go：服务注册（consul）。
//
// 服务端注册：把当前服务（service_name + advertise addr + tags）注册到 consul agent。
// 客户端发现走 clienthertz/discovery.go，不在本文件。
//
// 真实实现：
//
//	consulClient, _ := consulapi.NewClient(...)
//	r := consul.NewConsulRegister(consulClient, ...)
//	info := &registry.Info{ServiceName: ..., Addr: ..., Tags: ...}
//	server.WithRegistry(r, info)
package serverhertz

import (
	"github.com/cloudwego/hertz/pkg/app/server/registry"

	configpb "media_agent/hertz_gen/config"
)

// buildRegistry 解析 cfg.Consul，返回 (registrar, info)；
// 未启用时返回 (nil, nil)。
//
// 当前依赖（hertz-contrib/registry/consul + hashicorp/consul/api）尚未引入，
// 返回 nil 不影响 build；真实实现时替换为：
//
//	cc, err := consulapi.NewClient(&consulapi.Config{Address: c.GetAddress(), Scheme: c.GetScheme(), Token: c.GetToken()})
//	r := consul.NewConsulRegister(cc, consul.WithCheck(...))
//	info := &registry.Info{
//	    ServiceName: cfg.App.GetName(),
//	    Addr:        utils.NewNetAddr("tcp", net.JoinHostPort(c.GetServiceAddress(), strconv.Itoa(int(c.GetServicePort())))),
//	    Tags:        c.GetTags(),
//	    Weight:      registry.DefaultWeight,
//	}
//	return r, info
func buildRegistry(cfg *configpb.Config) (registry.Registry, *registry.Info) {
	c := cfg.GetConsul()
	if c == nil || !c.GetEnabled() {
		return nil, nil
	}
	// TODO(infra): 实接 hertz-contrib/registry/consul。
	return nil, nil
}

// RegisterService 在路由全部注册后再调用，触发同步注册到 consul。
// Hertz 已通过 server.WithRegistry 在启动时自动注册，本接口保留给手工触发场景。
func (s *ServerSuite) RegisterService() error {
	if s.registrar == nil || s.registryInfo == nil {
		return nil
	}
	return s.registrar.Register(s.registryInfo)
}

// DeregisterService 在 main shutdown 阶段调用。
func (s *ServerSuite) DeregisterService() error {
	if s.registrar == nil || s.registryInfo == nil {
		return nil
	}
	return s.registrar.Deregister(s.registryInfo)
}
