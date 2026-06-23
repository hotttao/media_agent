// Package clienthertz 是 Hertz 客户端治理能力唯一收口。
//
// 按客户端调用指南（contributing/client.md）§6：
//   - 只提供治理能力（client.Option / client.Middleware），不持有业务 client 实例，
//     不调用 hclient.NewClient（NewClient 方法除外，它只是装配便捷入口）；
//   - 实例化归 <app>/biz/shared/client。
//
// 对称 serverhertz：所有入口收口在 ClientSuite 上，不散落包级 Init/Apply 函数。
//
// 装配流程（调用方典型用法）：
//
//	baseSuite := clienthertz.BuildBaseSuite(cfg)              // 进程级基底（含完整 cfg）
//	shutdown, _ := baseSuite.InitObservability()              // logger + tracing + metrics
//	defer shutdown(context.Background())
//
//	appSuite, _ := baseSuite.ForApp("media_scheduler")        // 派生下游 app 级 suite
//	cli, _    := appSuite.NewClient()                         // NewClient + ApplyDiscovery + ApplyMiddlewares
//
//	// 或拆步用于 typed client（hertz_gen NewHertzClient）：
//	//   cli, _ := hclient.NewClient(appSuite.Options()...)
//	//   appSuite.ApplyDiscovery(cli)
//	//   appSuite.ApplyMiddlewares(cli)
package clienthertz

import (
	"context"
	"fmt"

	hclient "github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/common/config"

	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/observability"
)

// ClientSuite 聚合客户端治理能力。
//
// 由 BuildBaseSuite / BuildAppSuite 构造：
//   - BuildBaseSuite 填充进程级字段（cfg / consul_discovery / otel / prometheus）；
//   - BuildAppSuite / ForApp 在 base 副本上叠加 app 级 4 行为（timeout / retry /
//     circuit_breaker / rate_limit），每次返回独立副本——同一 app 的多个 service
//     共享同一份 app suite。
type ClientSuite struct {
	// 进程级
	cfg             *configpb.Config // 完整配置（供 InitObservability / ForApp）
	consulDiscovery *configpb.ConsulDiscoveryConfig
	otel            *configpb.OtelConfig
	prometheus      *configpb.PrometheusConfig

	// app 级（由 BuildAppSuite / ForApp 填充）
	appName        string
	appCfg         *configpb.AppClientConfig
	timeoutOpts    []config.ClientOption
	retryOpts      []config.ClientOption
	useDiscovery   bool
	loadBalancer   string
	circuitBreaker *configpb.CircuitBreakerConfig
	rateLimit      *configpb.RateLimitConfig
}

// InitObservability 进程级可观测性初始化（logger + tracing + metrics），对称 serverhertz。
//
// 顺序：logger → tracing provider → metrics。返回统一 shutdown closure（幂等安全）。
// 各段 enabled=false 时 no-op。调用方（main / biz/shared/client.Init / media-cli）defer 调用。
func (s *ClientSuite) InitObservability() (func(context.Context) error, error) {
	if s == nil || s.cfg == nil {
		return func(context.Context) error { return nil }, nil
	}
	cfg := s.cfg
	if err := observability.InitLogger(cfg.GetApp(), cfg.GetLog()); err != nil {
		return nil, fmt.Errorf("clienthertz: init logger: %w", err)
	}
	shutdown := observability.InitOTel(cfg.GetApp(), cfg.GetOtel())
	if err := observability.InitMetrics(cfg.GetPrometheus()); err != nil {
		return nil, fmt.Errorf("clienthertz: init metrics: %w", err)
	}
	return shutdown, nil
}

// ForApp 派生下游 app 级 suite：从 cfg.Client[appName] 取配置，调用 BuildAppSuite。
//
// 下游 app 未配置时返回 error（contributing/client.md §7.2：配置缺失不得静默忽略）。
func (s *ClientSuite) ForApp(appName string) (*ClientSuite, error) {
	if s == nil || s.cfg == nil {
		return nil, fmt.Errorf("clienthertz: base suite not initialized")
	}
	appCfg, ok := s.cfg.GetClient()[appName]
	if !ok || appCfg == nil {
		return nil, fmt.Errorf("clienthertz: downstream app %q not configured in client map", appName)
	}
	return BuildAppSuite(s, appName, appCfg), nil
}

// Options 组装该 suite 对应的 Hertz client.Option（discovery 由 ApplyDiscovery 单独挂）。
func (s *ClientSuite) Options() []config.ClientOption {
	opts := []config.ClientOption{}
	opts = append(opts, s.timeoutOpts...)
	opts = append(opts, s.retryOpts...)
	return opts
}

// Middlewares 返回 client middleware 构造器列表（tracing / metrics / ratelimit / circuitbreaker），
// 顺序见 client.md §6.3（tracing → metrics → ratelimit → circuitbreaker）。
// 由 ApplyMiddlewares 统一挂到 client；导出以支持顺序稳定性单测。
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

// ApplyMiddlewares 把 suite 的 middleware 挂到 client（见 Middlewares 顺序）。
func (s *ClientSuite) ApplyMiddlewares(cli *hclient.Client) {
	if cli == nil || s == nil {
		return
	}
	for _, mw := range s.Middlewares() {
		mw(cli)
	}
}

// ApplyDiscovery 把服务发现 middleware 挂到 client（若 suite 启用了 discovery）。
// discovery 依赖 consul resolver，通常先于其它 middleware。
func (s *ClientSuite) ApplyDiscovery(cli *hclient.Client) {
	if cli == nil || s == nil || !s.useDiscovery {
		return
	}
	applyDiscovery(cli, s.consulDiscovery, s.loadBalancer)
}

// NewClient 装配便捷入口：NewClient(Options()) + ApplyDiscovery + ApplyMiddlewares。
//
// 适用于使用裸 *hclient.Client 的场景（如 media-cli）。typed client（hertz_gen
// NewHertzClient）请用 Options() + ApplyDiscovery + ApplyMiddlewares 自行装配。
func (s *ClientSuite) NewClient() (*hclient.Client, error) {
	cli, err := hclient.NewClient(s.Options()...)
	if err != nil {
		return nil, err
	}
	s.ApplyDiscovery(cli)
	s.ApplyMiddlewares(cli)
	return cli, nil
}

// AppName 返回该 suite 绑定的下游 app 名（供资源命名）。
func (s *ClientSuite) AppName() string { return s.appName }

// UseDiscovery 返回该下游是否走服务发现。
func (s *ClientSuite) UseDiscovery() bool { return s.useDiscovery }

// ConsulDiscovery 返回进程级服务发现配置。
func (s *ClientSuite) ConsulDiscovery() *configpb.ConsulDiscoveryConfig { return s.consulDiscovery }

// LoadBalancer 返回负载均衡策略字符串。
func (s *ClientSuite) LoadBalancer() string { return s.loadBalancer }

// BuildBaseSuite 从完整配置构造进程级 ClientSuite 基底。
//
// 只填充进程级字段（cfg / consul_discovery / otel / prometheus）；
// app 级 4 行为由 BuildAppSuite / ForApp 在副本上叠加。
func BuildBaseSuite(cfg *configpb.Config) *ClientSuite {
	return &ClientSuite{
		cfg:             cfg,
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
	suite := *base // 拷贝 base（含 cfg / 进程级字段）
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
