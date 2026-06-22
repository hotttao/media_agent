// Package clienthertz 是 Hertz 客户端治理能力唯一收口。
//
// 按客户端调用指南（contributing/client-cn.md）§6：
//   - 只提供治理能力（client.Option / client.Middleware 工厂）；
//   - 不持有业务 client 实例，不调用 hclient.NewClient；
//   - 实例化归 <app>/biz/shared/client/。
//
// 装配流程（调用方典型用法）：
//
//	baseSuite := clienthertz.BuildBaseSuite(cfg)              // 进程级
//	appSuite  := clienthertz.BuildAppSuite(baseSuite, appCfg) // app 级（拷贝一次）
//	opts      := clienthertz.BuildOptions(appSuite)           // []client.Option
//	cli, _    := hclient.NewClient(opts...)
//	clienthertz.ApplyMiddlewares(cli, appSuite, "media-scheduler")
package clienthertz

import (
	hclient "github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/common/config"

	configpb "media_agent/hertz_gen/config"
)

// ClientSuite 聚合启动期所有 client 治理能力。
//
// 由 BuildBaseSuite / BuildAppSuite 构造：
//   - BuildBaseSuite 只填充进程级字段（discovery / otel / prometheus）；
//   - BuildAppSuite 在 base 副本上叠加 app 级 4 行为（timeout / retry /
//     circuit_breaker / rate_limit），每次调用返回一份独立副本，因为
//     4 行为是 app 级共享的——同一 app 的多个 service 用同一份 suite。
type ClientSuite struct {
	// 进程级
	consulDiscovery *configpb.ConsulDiscoveryConfig
	otel            *configpb.OtelConfig
	prometheus      *configpb.PrometheusConfig

	// app 级（由 BuildAppSuite 填充）
	appName        string
	appCfg         *configpb.AppClientConfig
	timeoutOpts    []config.ClientOption
	retryOpts      []config.ClientOption
	useDiscovery   bool
	loadBalancer   string
	circuitBreaker *configpb.CircuitBreakerConfig
	rateLimit      *configpb.RateLimitConfig
}

// Options 组装该 suite 对应的 Hertz client.Option 列表（discovery / loadbalance / timeout / retry）。
//
// 中间件不通过 option 传递——见 ApplyMiddlewares。
func (s *ClientSuite) Options() []config.ClientOption {
	opts := []config.ClientOption{}
	opts = append(opts, s.timeoutOpts...)
	opts = append(opts, s.retryOpts...)
	return opts
}

// Middlewares 返回该 suite 对应的 client middleware 列表（tracing / metrics / ratelimit / circuitbreaker）。
//
// 返回 middleware 构造器列表，由 ApplyMiddlewares 统一挂到 client。
// 这里返回已绑定配置的 closure，调用方不需要再传配置。
func (s *ClientSuite) Middlewares() []func(*hclient.Client) {
	mws := []func(*hclient.Client){}
	if s.otel != nil && s.otel.GetEnabled() {
		mws = append(mws, func(cli *hclient.Client) {
			applyTracingMiddleware(cli, s.otel)
		})
	}
	if s.prometheus != nil && s.prometheus.GetEnabled() {
		mws = append(mws, func(cli *hclient.Client) {
			applyMetricsMiddleware(cli, s.prometheus)
		})
	}
	if s.rateLimit != nil && s.rateLimit.GetEnabled() {
		mws = append(mws, func(cli *hclient.Client) {
			applyRateLimitMiddleware(cli, s.rateLimit, s.appName)
		})
	}
	if s.circuitBreaker != nil && s.circuitBreaker.GetEnabled() {
		mws = append(mws, func(cli *hclient.Client) {
			applyCircuitBreakerMiddleware(cli, s.circuitBreaker, s.appName)
		})
	}
	return mws
}

// AppName 返回该 suite 绑定的下游 app 名（供 biz/shared/client 命名资源）。
func (s *ClientSuite) AppName() string { return s.appName }

// UseDiscovery 返回该下游是否走服务发现。
func (s *ClientSuite) UseDiscovery() bool { return s.useDiscovery }

// ConsulDiscovery 返回进程级服务发现配置（供 biz/shared/client 决定是否挂 discovery middleware）。
func (s *ClientSuite) ConsulDiscovery() *configpb.ConsulDiscoveryConfig { return s.consulDiscovery }

// LoadBalancer 返回负载均衡策略字符串。
func (s *ClientSuite) LoadBalancer() string { return s.loadBalancer }

// BuildBaseSuite 从进程级配置构造 ClientSuite 基底。
//
// 只填充进程级字段（consul_discovery / otel / prometheus）；
// app 级 4 行为由 BuildAppSuite 在副本上叠加。
func BuildBaseSuite(cfg *configpb.Config) *ClientSuite {
	return &ClientSuite{
		consulDiscovery: cfg.GetConsulDiscovery(),
		otel:            cfg.GetOtel(),
		prometheus:      cfg.GetPrometheus(),
	}
}

// BuildAppSuite 在 base 副本上叠加 app 级配置，返回独立副本。
//
// 同一 app 的多个 service 应共享同一份 BuildAppSuite 结果——4 行为是 app 级的。
func BuildAppSuite(base *ClientSuite, appName string, appCfg *configpb.AppClientConfig) *ClientSuite {
	if base == nil {
		return nil
	}
	// 拷贝一份 base
	suite := *base
	suite.appName = appName
	suite.appCfg = appCfg
	suite.timeoutOpts = buildTimeoutOptions(appCfg.GetTimeout())
	suite.retryOpts = buildRetryOptions(appCfg.GetRetry())
	suite.useDiscovery = appCfg.GetUseDiscovery()
	suite.loadBalancer = pickLoadBalancer(appCfg.GetLoadBalance())
	suite.circuitBreaker = appCfg.GetCircuitBreaker()
	suite.rateLimit = appCfg.GetRateLimit()
	return &suite
}

// BuildOptions 是 s.Options() 的便捷别名，供 biz/shared/client 直接传给 hclient.NewClient。
func BuildOptions(s *ClientSuite) []config.ClientOption {
	if s == nil {
		return nil
	}
	return s.Options()
}

// ApplyMiddlewares 把 suite 的 middleware 按 §6.3 顺序挂到 client。
//
// 顺序（外→内）：tracing → metrics → ratelimit → circuitbreaker → request
// discovery 不作为普通 middleware，由 biz/shared/client 在 NewClient 后单独 cli.Use。
func ApplyMiddlewares(cli *hclient.Client, s *ClientSuite) {
	if cli == nil || s == nil {
		return
	}
	for _, mw := range s.Middlewares() {
		mw(cli)
	}
}

// ApplyDiscovery 把服务发现 middleware 挂到 client（若 suite 启用了 discovery）。
//
// 单独抽出来是因为 discovery 依赖 consul resolver，且通常需要先于其它 middleware。
func ApplyDiscovery(cli *hclient.Client, s *ClientSuite) {
	if cli == nil || s == nil || !s.useDiscovery {
		return
	}
	applyDiscovery(cli, s.consulDiscovery, s.loadBalancer)
}
