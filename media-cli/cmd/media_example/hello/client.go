// client.go — 共享的 media_example HelloService typed client。
//
// 设计动机：
//   - hello service 后续会加 method1 / method2 / method3 等多个 RPC；
//   - 客户端对象昂贵（带 otel middleware、连接池等），每个 method 不应各自 new；
//   - 这里 Init(cfg) 在启动期构造一次，method 子命令通过 Client() 复用。
//
// 共享 client 模式（同一下游 app 多 service 复用一个 *hclient.Client）：
//   - clienthertz.NewClientSuiteFromConfig(cfg) → NewAppClientSuite("media_example") → NewClient()
//     产出一个已烘焙 timeout/retry/discovery/tracing/metrics/circuitbreaker 的共享 client；
//   - typed client 通过 WithHertzClient(shared) 复用它，不再各自 new；
//   - 用 WithHertzClient 后不要再传 WithHertzClientOption / WithHertzClientMiddleware
//     （治理已烘焙进 shared）。
//
// 配置来源（避免"乱倒配置"）：
//   - main.go 加载 cfg 后调 hello.Init(cfg)：本文件从 cfg.GetClient()["media_example"]
//     读 base_domain / timeout / retry / circuit_breaker，构造 typed client；
//   - cfg 本身不持有；后续方法命令只读包级变量 helloCli；
//   - cfg.GetClient()["media_example"] 缺失或 enabled=false 时 Init log.Fatalf 退出，
//     避免方法运行时 nil pointer。
package hello

import (
	"log"

	helloclient "media_agent/hertz_gen/media_example/hello_service"
	"media_agent/hertz_infra/clienthertz"
	config "media_agent/hertz_infra/config"
)

// helloCli 是 Init 构造的共享 typed client，供 method1.go 等复用。
var helloCli helloclient.Client

// Init 由 main.go 在加载 cfg 完成后调用，构造 helloCli。
//
// cfg.Client["media_example"] 必须 enabled=true 且有 base_domain；
// 否则 log.Fatalf 终止启动（避免后续 method 调用 nil pointer）。
func Init(cfg *config.Config) {
	cliCfg := cfg.GetClient()["media_example"]
	if cliCfg == nil || !cliCfg.GetEnabled() {
		log.Fatalf("hello: client.media_example 未配置或 enabled=false（请检查 conf/<env>.yaml）")
	}
	host := cliCfg.GetBaseDomain()
	if host == "" {
		log.Fatalf("hello: client.media_example.base_domain 为空")
	}

	// 共享 client：app 级 suite → 单个 *hclient.Client → 注入 typed client。
	base := clienthertz.NewClientSuiteFromConfig(cfg)
	appSuite, err := base.NewAppClientSuite("media_example")
	if err != nil {
		log.Fatalf("hello: derive downstream suite: %v", err)
	}
	shared, err := appSuite.NewClient()
	if err != nil {
		log.Fatalf("hello: build shared client: %v", err)
	}

	helloCli, err = helloclient.NewHelloServiceClient(host, helloclient.WithHertzClient(shared))
	if err != nil {
		log.Fatalf("hello: 创建 typed client 失败: %v", err)
	}
}

// Client 返回 Init 构造的共享 typed client；method1.go 等方法子命令调用。
func Client() helloclient.Client {
	return helloCli
}
