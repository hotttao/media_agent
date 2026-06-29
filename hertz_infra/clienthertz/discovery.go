// discovery.go：客户端服务发现。
//
// 实装（对齐 hertz-contrib/registry/consul）：
//
//	cc, _ := consulapi.NewClient(&consulapi.Config{Address, Scheme, Token})
//	r := consul.NewConsulResolver(cc)
//	cli.Use(sd.Discovery(r, sd.WithLoadBalanceOptions(lb, opts)))
//
// 注意：sd.Discovery 仅在请求带 config.WithSD(true) 时才解析实例地址
// （middleware 内 req.Options().IsSD() 判定）。这是 RequestOption，不能烘焙进 client，
// 故开启服务发现时调用方需：host=http://<service_name> 且每次 RPC 传 config.WithSD(true)。
// media-cli 测试路径 consul_discovery.enabled=false，不受影响。
package clienthertz

import (
	"log"
	"time"

	hclient "github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/app/client/loadbalance"
	sd "github.com/cloudwego/hertz/pkg/app/middlewares/client/sd"
	consul "github.com/hertz-contrib/registry/consul"
	consulapi "github.com/hashicorp/consul/api"

	configpb "media_agent/hertz_gen/config"
)

func applyDiscovery(cli *hclient.Client, cd *configpb.ConsulDiscoveryConfig, lb string) {
	if cli == nil || cd == nil || !cd.GetEnabled() {
		return
	}
	cc, err := consulapi.NewClient(&consulapi.Config{
		Address: cd.GetAddress(),
		Scheme:  cd.GetScheme(),
		Token:   cd.GetToken(),
	})
	if err != nil {
		log.Printf("clienthertz: build consul client for discovery: %v", err)
		return
	}
	resolver := consul.NewConsulResolver(cc)

	// hertz 核心只自带 NewWeightedBalancer（加权随机）；round_robin / consistent_hash
	// 需 hertz-contrib/loadbalance 或自定义 Loadbalancer，当前统一回退加权随机
	// （consul 注册权重恒定，加权随机分布上近似轮询）。
	balancer := loadbalance.NewWeightedBalancer()
	if lb != "" && lb != "weighted_random" {
		log.Printf("clienthertz: load_balance %q not natively supported, fallback to weighted_random", lb)
	}

	opts := loadbalance.DefaultLbOpts
	if sec := cd.GetRefreshIntervalSeconds(); sec > 0 {
		opts.RefreshInterval = time.Duration(sec) * time.Second
	}
	// datacenter 当前忽略：NewConsulResolver.Resolve 不透传 QueryOptions，
	// 需自定义 resolver 才能按 datacenter 过滤。

	cli.Use(sd.Discovery(resolver, sd.WithLoadBalanceOptions(balancer, opts)))
}
