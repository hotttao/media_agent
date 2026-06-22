// Package serverhertz 是服务端统一治理层。
//
// main.go 仅需：
//
//	loader, _ := config.NewLoader()
//	suite := serverhertz.NewServerSuite(loader)
//	defer loader.Close()
//	loader.Attach(suite.KVSource())
//	loader.Watch()
//	h := serverhertz.NewServer(suite)
//	register(h) // 生成的路由注册
//	suite.RegisterService() // 服务注册
//	h.Spin()
//
// 所有跨服务复用的服务端能力收口在本包，禁止下沉到 biz/。
package serverhertz

import (
	"log"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/app/server/registry"
	hertzconfig "github.com/cloudwego/hertz/pkg/common/config"

	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/config"
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

	// 可热更新能力组件在此处自行 loader.Subscribe。
	// 例（限流规则热加载）：
	//   if rl := cfg.GetRateLimit(); rl != nil && rl.GetEnabled() {
	//       loader.Subscribe(func(newCfg *configpb.Config) {
	//           applyRateLimitRules(newCfg)
	//       })
	//   }
	_ = cfg

	return s
}

// Options 返回拼好的 Hertz server.Option 列表。
func (s *ServerSuite) Options() []hertzconfig.Option { return s.options }

// Config 返回当前配置快照（转发 loader.Current()）。
// 仅供 NewServer 等 serverhertz 内部消费，禁止业务层拿。
func (s *ServerSuite) Config() *configpb.Config { return s.loader.Current() }

// Loader 暴露底层 loader，仅供 serverhertz 内部组件订阅使用。
// 业务层应直接 import hertz_infra/config 自己订阅，不通过本方法。
func (s *ServerSuite) Loader() *config.Loader { return s.loader }

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
