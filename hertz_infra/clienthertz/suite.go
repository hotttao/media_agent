// Package clienthertz 是 Hertz 客户端治理能力唯一收口。
//
// 按客户端调用指南（contributing/client.md）§6：
//   - 只提供治理能力（client.Option / client.Middleware），不持有业务 client 实例，
//     不调用 hclient.NewClient（NewClient 方法除外，它只是装配便捷入口）；
//   - 实例化归 <app>/biz/shared/client。
//
// 对称 serverhertz：ClientSuite 持有 *config.Loader（不持有 cfg 快照），
// NewClientSuite 收 loader 不收 cfg；可热更新能力组件在 ForApp 内部
// loader.Subscribe 自行订阅，ClientSuite 不做桥接。
//
// 字段分三类（见 ClientSuite 注释）：
//   - 进程级配置（otel/prometheus/consul_discovery）：不存，用 current() 实时读，避免冗余；
//   - 派生+固化（timeout/retry/discovery/lb）：baked 进 client，NewClient 后不可变；
//   - 可热更新（circuit_breaker 规则）：atomic.Pointer + ForApp 内订阅刷新。
//
// 治理职责（见 docs/plan/observability.md）：限流只在 server 侧（serverhertz/ratelimit），
// 熔断只在 client 侧（本包 circuitbreaker.go）。client 不实现限流。
//
// 装配流程（调用方典型用法）：
//
//	baseSuite := clienthertz.NewClientSuite(loader)          // 收 loader 不收 cfg
//	shutdown, _ := baseSuite.InitObservability()             // logger + tracing + metrics
//	defer shutdown(context.Background())
//
//	appSuite, _ := baseSuite.NewAppClientSuite("media_scheduler")       // 派生下游 app 级 suite + 订阅
//	cli, _    := appSuite.NewClient()                        // NewClient + ApplyDiscovery + ApplyMiddlewares
package clienthertz

import (
	"context"
	"fmt"
	"sync/atomic"

	hclient "github.com/cloudwego/hertz/pkg/app/client"
	hertzconfig "github.com/cloudwego/hertz/pkg/common/config"

	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/globalhertz"
)

// ClientSuite 聚合客户端治理能力，对称 serverhertz.ServerSuite。
//
// 持有 *config.Loader（不持有 cfg 快照）：
//   - 启动期固化的能力（app 级 timeout/retry/discovery/lb、middleware 顺序）在
//     ForApp 内部一次性消费 current()——这些是 client.Option，NewClient 后烘焙进 client 不可变；
//   - 可热更新能力（限流 / 熔断规则）在 ForApp 内部 loader.Subscribe 自行订阅，
//     写入 atomic.Pointer，ClientSuite 不做桥接。
type ClientSuite struct {
	// 进程级
	loader    *config.Loader   // 优先：转发 loader.Current() + 订阅
	cfg       *configpb.Config // 兜底：无 loader 时（media-cli 内存 cfg）的静态快照
	appName   string           // 当前 suite 绑定的下游 app（base suite 为空）
	callerApp string           // 当前 caller 服务名（app.name），用于熔断资源名 "<caller>-><callee>"

	// 派生 + 固化（ForApp 一次性计算，NewClient 时烘焙进 client，不可热更新）
	timeoutOpts  []hertzconfig.ClientOption
	retryOpts    []hertzconfig.ClientOption
	useDiscovery bool
	loadBalancer string

	// 可热更新规则（atomic，放在堆 holder 内避免拷贝 ClientSuite 触发 noCopy）。
	// ForApp 分配 holder 并 Store 初始规则；loader.Subscribe 回调刷新 + reloadRules() 重载。
	// base suite 为 nil。
	rules *ruleState
}

// ruleState 持有可热更新的熔断规则快照（atomic，并发安全读）。
// 仅 circuit_breaker（限流不在 client 侧实现）。
type ruleState struct {
	circuitBreaker atomic.Pointer[configpb.CircuitBreakerConfig]
}

// current 返回当前配置快照：优先 loader.Current()，无 loader 时用静态 cfg。
func (s *ClientSuite) current() *configpb.Config {
	if s == nil {
		return nil
	}
	if s.loader != nil {
		return s.loader.Current()
	}
	return s.cfg
}

// NewClientSuite 基于 *config.Loader 构造进程级 ClientSuite 基底，对称 serverhertz.NewServerSuite。
//
// 收 loader 不收 cfg。进程级字段（otel/prometheus/consul_discovery）不存，按需 current() 读；
// 可热更新能力组件在 ForApp（知道下游 app 名后）内部 loader.Subscribe 订阅。
func NewClientSuite(loader *config.Loader) *ClientSuite {
	if loader == nil {
		return nil
	}
	return &ClientSuite{loader: loader, cfg: loader.Current()}
}

// InitObservability 进程级可观测性初始化（logger + tracing + metrics），对称 serverhertz。
//
// 顺序：logger → tracing provider → metrics。返回统一 shutdown closure（幂等安全）。
// 各段 enabled=false 时 no-op。
func (s *ClientSuite) InitObservability() (func(context.Context) error, error) {
	cfg := s.current()
	if cfg == nil {
		return func(context.Context) error { return nil }, nil
	}
	if err := globalhertz.InitLogger(cfg.GetApp(), cfg.GetLog()); err != nil {
		return nil, fmt.Errorf("clienthertz: init logger: %w", err)
	}
	shutdown := globalhertz.InitOTel(cfg.GetApp(), cfg.GetOtel())
	// client 不暴露 /metrics（pull 端点归 server），故无需 InitMetrics。
	return shutdown, nil
}

// NewAppClientSuite 派生下游 app 级 suite：从 current().Client[appName] 取配置，固化 app 级行为 + 订阅热更新。
//
// 下游 app 未配置时返回 error（contributing/client.md §7.2：配置缺失不得静默忽略）。
//   - timeout/retry/discovery/lb：启动期固化（client.Option，NewClient 后不可变）；
//   - circuit_breaker：atomic 存当前规则，并 loader.Subscribe 订阅刷新 +
//     reloadRules() 重载 sentinel 全局熔断规则（contributing/server.md §配置订阅职责切分）。
//
// 同一 app 的多个 service 应共享同一份 NewAppClientSuite 结果 + 同一个 NewClient() 产物——
// 行为是 app 级的，连接池/中间件链也按 app 复用（见调用方共享 client 模式）。
func (s *ClientSuite) NewAppClientSuite(appName string) (*ClientSuite, error) {
	cfg := s.current()
	if cfg == nil {
		return nil, fmt.Errorf("clienthertz: base suite not initialized")
	}
	appCfg, ok := cfg.GetClient()[appName]
	if !ok || appCfg == nil {
		return nil, fmt.Errorf("clienthertz: downstream app %q not configured in client map", appName)
	}

	suite := *s // 拷贝 base（loader / cfg；rules 为 nil，base 未持有）
	suite.appName = appName
	if a := cfg.GetApp(); a != nil {
		suite.callerApp = a.GetName()
	}
	suite.timeoutOpts = buildTimeoutOptions(appCfg.GetTimeout())
	suite.retryOpts = buildRetryOptions(appCfg.GetRetry())
	suite.useDiscovery = appCfg.GetUseDiscovery()
	suite.loadBalancer = pickLoadBalancer(appCfg.GetLoadBalance())
	suite.rules = &ruleState{}
	suite.rules.circuitBreaker.Store(appCfg.GetCircuitBreaker())
	// 启动期下发熔断规则（懒初始化 sentinel）。
	_ = loadCircuitBreakerRules(appCfg.GetCircuitBreaker(), suite.callerApp, appName)

	// 可热更新能力组件在此自行 loader.Subscribe（ClientSuite 不做桥接）。
	// 配置中心（Consul KV）变更 → 刷新 atomic 熔断规则 + 重载 sentinel 全局规则。
	if s.loader != nil {
		s.loader.Subscribe(func(newCfg *configpb.Config) {
			ac := newCfg.GetClient()[appName]
			if ac == nil {
				return
			}
			suite.rules.circuitBreaker.Store(ac.GetCircuitBreaker())
			caller := ""
			if a := newCfg.GetApp(); a != nil {
				caller = a.GetName()
			}
			_ = loadCircuitBreakerRules(ac.GetCircuitBreaker(), caller, appName)
		})
	}
	return &suite, nil
}

// Options 组装该 suite 对应的 Hertz client.Option（discovery 由 ApplyDiscovery 单独挂）。
func (s *ClientSuite) Options() []hertzconfig.ClientOption {
	opts := []hertzconfig.ClientOption{}
	opts = append(opts, s.timeoutOpts...)
	opts = append(opts, s.retryOpts...)
	return opts
}

// Middlewares 返回 client per-request middleware 列表（tracing / metrics / circuitbreaker），
// 顺序见 client.md §6.3。返回 hertz_client.Middleware，可同时用于：
//   - 裸 client：cli.Use(mws...)
//   - 生成 typed client：NewXxxClient(host, WithHertzClientMiddleware(mws...))
//
// 进程级 otel/prometheus 实时读 current()；app 级 circuitbreaker 读 atomic 当前规则。
// （限流不在 client 侧实现，归 serverhertz。）
func (s *ClientSuite) Middlewares() []hclient.Middleware {
	cfg := s.current()
	mws := []hclient.Middleware{}
	if cfg != nil {
		if mw := tracingMiddleware(cfg.GetOtel()); mw != nil {
			mws = append(mws, mw)
		}
		if mw := metricsMiddleware(cfg.GetPrometheus()); mw != nil {
			mws = append(mws, mw)
		}
	}
	if s.rules != nil {
		if cb := s.rules.circuitBreaker.Load(); cb != nil && cb.GetEnabled() {
			mws = append(mws, circuitBreakerMiddleware(cb, s.callerApp, s.appName))
		}
	}
	return mws
}

// ApplyMiddlewares 把 suite 的 per-request middleware 挂到裸 client（cli.Use）。
// 生成 typed client 请直接用 Middlewares() + WithHertzClientMiddleware，不走本方法。
func (s *ClientSuite) ApplyMiddlewares(cli *hclient.Client) {
	if cli == nil || s == nil {
		return
	}
	mws := s.Middlewares()
	if len(mws) > 0 {
		cli.Use(mws...)
	}
}

// ApplyDiscovery 把服务发现 middleware 挂到 client（若 suite 启用了 discovery）。
// consul_discovery 实时读 current()。
func (s *ClientSuite) ApplyDiscovery(cli *hclient.Client) {
	if cli == nil || s == nil || !s.useDiscovery {
		return
	}
	if cd := s.current().GetConsulDiscovery(); cd != nil {
		applyDiscovery(cli, cd, s.loadBalancer)
	}
}

// NewClient 装配便捷入口：NewClient(Options()) + ApplyDiscovery + ApplyMiddlewares。
func (s *ClientSuite) NewClient() (*hclient.Client, error) {
	cli, err := hclient.NewClient(s.Options()...)
	if err != nil {
		return nil, err
	}
	s.ApplyDiscovery(cli)
	s.ApplyMiddlewares(cli)
	return cli, nil
}

// Loader 暴露底层 loader，仅供 clienthertz 内部组件订阅使用。
func (s *ClientSuite) Loader() *config.Loader { return s.loader }

// AppName 返回该 suite 绑定的下游 app 名（供资源命名）。
func (s *ClientSuite) AppName() string { return s.appName }

// UseDiscovery 返回该下游是否走服务发现。
func (s *ClientSuite) UseDiscovery() bool { return s.useDiscovery }

// ConsulDiscovery 返回进程级服务发现配置（实时读 current()）。
func (s *ClientSuite) ConsulDiscovery() *configpb.ConsulDiscoveryConfig {
	if cfg := s.current(); cfg != nil {
		return cfg.GetConsulDiscovery()
	}
	return nil
}

// LoadBalancer 返回负载均衡策略字符串。
func (s *ClientSuite) LoadBalancer() string { return s.loadBalancer }

// CircuitBreaker 返回当前熔断规则快照（atomic 读取，供内省 / 单测）。
func (s *ClientSuite) CircuitBreaker() *configpb.CircuitBreakerConfig {
	if s == nil || s.rules == nil {
		return nil
	}
	return s.rules.circuitBreaker.Load()
}

// NewClientSuiteFromConfig 从完整 *configpb.Config 构造 ClientSuite（无 loader，不订阅）。
//
// 仅供无 loader 的场景（如 media-cli 用内存 cfg 装配测试 client）。生产服务应走
// NewClientSuite(loader)，以便可热更新能力组件按需订阅。
// 注意：此路径无 loader → ForApp 不会订阅，限流/熔断规则为启动期静态快照。
func NewClientSuiteFromConfig(cfg *configpb.Config) *ClientSuite {
	if cfg == nil {
		return nil
	}
	return &ClientSuite{cfg: cfg} // 静态快照（无 loader，不订阅）
}
