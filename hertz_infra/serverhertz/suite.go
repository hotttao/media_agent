// Package serverhertz 是服务端统一治理层。
//
// 所有服务端能力收口在 ServerSuite 上，main.go 仅按顺序调用其方法，不接触散落的
// 包级 Init / Register 函数：
//
//	loader, _ := config.NewLoader()
//	suite := serverhertz.NewServerSuite(loader)
//	defer loader.Close()
//	loader.Attach(suite.KVSource())
//	loader.Watch()
//
//	shutdown, _ := suite.InitObservability()   // logger + tracing + metrics
//	defer shutdown(context.Background())
//
//	h := suite.NewServer()                     // server + 全局 middleware
//	register(h)                                // 生成的业务路由
//	suite.RegisterRoutes(h)                    // 运维端点（health/pprof/metrics）
//	suite.RegisterService()                    // 服务注册
//	defer suite.DeregisterService()
//	h.Spin()
//
// 每个能力（tracing/metrics/health/pprof/...）拆到独立文件，只暴露私有构造函数
// 供 ServerSuite 调用；对外入口只有 ServerSuite 的方法。
package serverhertz

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/app/server/registry"
	hertzconfig "github.com/cloudwego/hertz/pkg/common/config"

	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/globalhertz"
)

// ServerSuite 聚合服务端启动期需要的全部 hertz options 与 middleware 顺序约束，
// 由 NewServerSuite(loader) 统一构造，main.go 不再分散组装。
//
// ServerSuite 持有 *config.Loader 而非快照 cfg：
//   - 启动期固化的能力（server options / middleware chain / registry）在
//     NewServerSuite / NewServer 内部一次性消费 loader.Current()；
//   - 可热更新的能力组件（限流规则 / 熔断规则）在 NewServerSuite 内部
//     loader.Subscribe 自己订阅，不通过 ServerSuite 桥接。
type ServerSuite struct {
	loader *config.Loader

	// hertz server 启动 options（监听地址、超时、注册中心等），启动期一次性消费
	options []hertzconfig.Option

	// 注册器：suite.RegisterService 时使用；Hertz server 启动期已通过 options 注入
	registrar registry.Registry
	// 注册元数据：与 registrar 配套
	registryInfo *registry.Info
}

// NewServerSuite 基于 config.Loader 构造一份 ServerSuite。
//
// 约束：
//   - 不读 ENV，不做 IO，纯函数；副作用集中到 NewServer / RegisterService。
//   - loader.Current() 中各能力 segment 的 enabled=false 时，对应 option / middleware 不挂载。
//   - 可热更新能力组件在此处 loader.Subscribe 自行订阅。
func NewServerSuite(loader *config.Loader) *ServerSuite {
	cfg := loader.Current()
	s := &ServerSuite{loader: loader}

	// server 监听地址 / 超时（启动期固化，不订阅）
	s.options = append(s.options, buildServerOptions(cfg)...)

	// 服务注册（registry + Info，启动期固化）
	if reg, info := buildRegistry(cfg); reg != nil {
		s.registrar = reg
		s.registryInfo = info
		s.options = append(s.options, server.WithRegistry(reg, info))
	}

	// 全局 tracer：metrics、tracing 都通过 server tracer 接入（启动期固化）
	s.options = append(s.options, buildTracerOptions(cfg)...)

	// 限流规则：启动期下发 + 订阅热更新（contributing/server.md §配置订阅职责切分）。
	// middleware 链在 NewServer 时固化（sentinel slot chain 注册一次），规则本身可热更。
	_ = loadFlowRules(cfg.GetRateLimit())
	loader.Subscribe(func(newCfg *configpb.Config) {
		_ = loadFlowRules(newCfg.GetRateLimit())
	})

	return s
}

// Options 返回拼好的 Hertz server.Option 列表。
func (s *ServerSuite) Options() []hertzconfig.Option { return s.options }

// Middlewares 返回按约定顺序组装的全局 middleware 列表（见 middleware.go）。
// 供 NewServer 挂载；导出以支持顺序稳定性单测（contributing/server.md §12.1）。
func (s *ServerSuite) Middlewares() []app.HandlerFunc {
	return buildMiddlewareChain(s.Config())
}

// Config 返回当前配置快照（转发 loader.Current()）。
// 仅供 serverhertz 内部消费，禁止业务层拿。
func (s *ServerSuite) Config() *configpb.Config { return s.loader.Current() }

// Loader 暴露底层 loader，仅供 serverhertz 内部组件订阅使用。
// 业务层应直接 import hertz_infra/config 自己订阅，不通过本方法。
func (s *ServerSuite) Loader() *config.Loader { return s.loader }

// InitObservability 进程级可观测性初始化的唯一入口：logger + tracing provider + metrics 状态。
//
// 顺序：先 logger（替换 hlog，后续组件日志走结构化输出）→ tracing provider → metrics。
// 返回统一的 shutdown closure（main defer 调用，幂等安全）；任一失败返回 err 由调用方 fatal。
// 各段 enabled=false 时对应能力 no-op，不报错（enabled 语义）。
func (s *ServerSuite) InitObservability() (func(context.Context) error, error) {
	cfg := s.Config()
	if err := globalhertz.InitLogger(cfg.GetApp(), cfg.GetLog()); err != nil {
		return nil, fmt.Errorf("serverhertz: init logger: %w", err)
	}
	shutdownTracing := globalhertz.InitOTel(cfg.GetApp(), cfg.GetOtel())
	if err := initMetrics(cfg.GetPrometheus()); err != nil {
		return nil, fmt.Errorf("serverhertz: init metrics: %w", err)
	}
	return shutdownTracing, nil
}

// NewServer 创建 Hertz server 并挂载全局 middleware（启动期一次性消费 Config()）。
//
// 中间件顺序见 middleware.go。运维端点不在本方法注册——由 RegisterRoutes 统一注册。
func (s *ServerSuite) NewServer() *server.Hertz {
	h := server.New(s.Options()...)
	for _, mw := range s.Middlewares() {
		h.Use(mw)
	}
	return h
}

// RegisterRoutes 统一注册运维端点（health / pprof / metrics route），按各段 enabled 门控。
//
// 必须在业务路由注册之后调用（hertz 路由按注册顺序匹配，运维端点路径不与业务冲突，
// 顺序无强约束，但约定业务路由先行）。metrics route 当前为 no-op（monitor-prometheus
// 自管理 :9091，不在 Hertz server 暴露），保留入口以便未来切换后端。
func (s *ServerSuite) RegisterRoutes(h *server.Hertz) {
	cfg := s.Config()
	registerHealthRoutes(h, cfg)
	registerPProfRoutes(h, cfg)
	registerMetricsRoute(h, cfg)
}

// KVSource 根据 loader.Current().consul_kv.enabled 返回一个 config.Source。
//
// enabled=false 返回 nil（调用方可以无条件 loader.Attach(suite.KVSource())，
// nil 时 Loader.Attach 无操作）。
//
// 职责分工：
//   - config.Loader 只依赖 config.Source 接口，不知道 Consul 存在；
//   - 本方法负责构造具体 Source 实现（consulKVSource），由调用方 Attach 到 Loader。
//
// 与 registry.go 消费 cfg.GetConsul() 是同一 pattern。
func (s *ServerSuite) KVSource() config.Source {
	kvCfg := s.loader.Current().GetConsulKv()
	if kvCfg == nil || !kvCfg.GetEnabled() {
		return nil
	}
	src, err := newConsulKVSource(kvCfg)
	if err != nil {
		log.Printf("serverhertz: consul kv source (skip): %v", err)
		return nil
	}
	return src
}
