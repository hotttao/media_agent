// registry.go：服务注册（consul）。
//
// 服务端注册：把当前服务（service_name + advertise addr + tags）注册到 consul agent。
// 客户端发现走 clienthertz/discovery.go，不在本文件。
//
// 实装（对齐 hertz-contrib/registry/consul）：
//
//	cc, _ := consulapi.NewClient(&consulapi.Config{Address, Scheme, Token})
//	r := consul.NewConsulRegister(cc, consul.WithCheck(check))
//	info := &registry.Info{ServiceName, Addr, Tags, Weight}
//	server.WithRegistry(r, info)
package serverhertz

import (
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server/registry"
	"github.com/cloudwego/hertz/pkg/common/utils"
	consul "github.com/hertz-contrib/registry/consul"
	consulapi "github.com/hashicorp/consul/api"

	configpb "media_agent/hertz_gen/config"
)

// buildRegistry 解析 cfg.Consul，返回 (registrar, info)；未启用时返回 (nil, nil)。
//
// health check 只设 Interval/Timeout/DeregisterCriticalServiceAfter：
// hertz-contrib/registry/consul 的 Register 会强制 check.TCP=host:port，
// 若再设 check.HTTP 会让单个 AgentServiceCheck 同时含 http+tcp，被 Consul 拒绝
// （"check may have at most one of http/tcp/grpc/..."）。故 health_check_path 当前保留但不使用。
//
// registry.Info.Tags 是 map[string]string，而 proto tags 是 repeated string；
// tagsSliceToMap 按 contrib convTagMapToSlice/splitTags 的 ":" 约定互转
// （key 不得含 ":"，无 ":" 则 value=""）。
func buildRegistry(cfg *configpb.Config) (registry.Registry, *registry.Info) {
	c := cfg.GetConsul()
	if c == nil || !c.GetEnabled() {
		return nil, nil
	}
	cc, err := consulapi.NewClient(&consulapi.Config{
		Address: c.GetAddress(),
		Scheme:  c.GetScheme(),
		Token:   c.GetToken(),
	})
	if err != nil {
		// 与 KVSource() 一致：构造失败按「未启用」处理，不阻断启动。
		log.Printf("serverhertz: build consul client: %v", err)
		return nil, nil
	}

	check := &consulapi.AgentServiceCheck{
		Interval:                       checkDurationString(c.GetHealthCheckIntervalSeconds(), 5),
		Timeout:                        checkDurationString(c.GetHealthCheckTimeoutSeconds(), 5),
		DeregisterCriticalServiceAfter: checkDurationString(c.GetDeregisterCriticalAfterSeconds(), 60),
	}
	reg := consul.NewConsulRegister(cc, consul.WithCheck(check))

	addr := net.JoinHostPort(c.GetServiceAddress(), strconv.Itoa(int(c.GetServicePort())))
	info := &registry.Info{
		ServiceName: cfg.GetApp().GetName(),
		Addr:        utils.NewNetAddr("tcp", addr),
		Tags:        tagsSliceToMap(c.GetTags()),
		Weight:      registry.DefaultWeight,
	}
	return reg, info
}

// tagsSliceToMap 把 proto repeated string tags 转成 registry.Info.Tags (map[string]string)。
// 约定对齐 hertz-contrib/registry/consul：每个 tag 按 ":" 拆成 key/value，无 ":" 则 value=""。
// key 含 ":" 会被 contrib 拒绝，这里跳过非法 tag（避免注册整体失败）。
func tagsSliceToMap(tags []string) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		if t == "" {
			continue
		}
		if strings.Contains(t, ":") {
			parts := strings.SplitN(t, ":", 2)
			if strings.Contains(parts[0], ":") {
				continue // 非法 key，跳过
			}
			m[parts[0]] = parts[1]
		} else {
			m[t] = ""
		}
	}
	return m
}

// secondsToDuration 把秒数转成 consul 接受的 "5s" 形式；<=0 用 fallback。
func checkDurationString(sec, fallback int32) string {
	if sec <= 0 {
		sec = fallback
	}
	return (time.Duration(sec) * time.Second).String()
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
