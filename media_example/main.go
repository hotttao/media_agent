// main.go 是 Hertz 程序入口。
//
// 所有服务端治理能力收口在 ServerSuite 上，main.go 只按顺序调用其方法
// （详见 hertz_infra/serverhertz/suite.go 包注释）：
//  1. config.NewLoader 加载配置；
//  2. serverhertz.NewServerSuite 构造 ServerSuite；
//  3. loader.Attach(KVSource) + loader.Watch 启动配置热更新；
//  4. suite.InitObservability 进程级可观测性初始化（logger + tracing + metrics）；
//  5. client.Init 初始化下游 client 装配层；
//  6. suite.NewServer 创建 server + 挂全局 middleware；
//  7. InitDI() 注入 handler 端 service 实例（必须在 register 之前）；
//  8. register(h) 注册生成的业务路由；
//  9. suite.RegisterRoutes 注册运维端点（health / pprof / metrics）；
//  10. suite.RegisterService 注册服务，defer 反注册；
//  11. h.Spin。
//
// main.go 只做启动编排，不直接拼 options / middleware / 注册中心。
package main

import (
	"context"
	"log"

	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/media_example/biz/shared/client"
)

func main() {
	// 1. 创建 config Loader（一次调用闭环：.env + APP_ENV → 读文件 + 首次加载）。
	//    Loader 内部自动创建 file source，但不启动 watch——watch 由 Watch() 显式启动。
	loader, err := config.NewLoader()
	if err != nil {
		log.Fatalf("init config loader: %v", err)
	}
	defer loader.Close()

	// 2. 构造 ServerSuite（收 loader 不收 cfg；可热更新能力组件在 suite 内部自行 loader.Subscribe）。
	suite := serverhertz.NewServerSuite(loader)

	// 3. Attach Consul KV source（consul_kv.enabled=false 时 KVSource() 返回 nil，Attach 无操作）。
	if err := loader.Attach(suite.KVSource()); err != nil {
		log.Fatalf("attach kv source: %v", err)
	}

	// 4. 启动所有 source 的 watch（文件 fsnotify + Consul KV blocking-query）。
	if err := loader.Watch(); err != nil {
		log.Fatalf("start config watch: %v", err)
	}

	// 5. 进程级可观测性初始化（logger + tracing + metrics），必须在 NewServer 之前完成。
	shutdownObs, err := suite.InitObservability()
	if err != nil {
		log.Fatalf("init observability: %v", err)
	}
	defer shutdownObs(context.Background())

	// 6. 初始化下游 client 装配层（当前模板无下游，baseSuite 仅做进程级装配）。
	if _, err := client.Init(loader.Current()); err != nil {
		log.Fatalf("init clients: %v", err)
	}

	// 7. 创建 Hertz server + 挂全局 middleware。
	h := suite.NewServer()

	// 8. 依赖注入：把 service 装配到 handler 包级变量。
	//    必须在 register(h) 之前完成，确保首个请求到达时 HelloSvc 非 nil。
	InitDI()

	// 9. 注册生成的业务路由（hz update 由 router_gen.go + biz/router/register.go 提供）。
	register(h)

	// 10. 注册运维端点（health / pprof / metrics route），按 enabled 门控。
	suite.RegisterRoutes(h)

	// 11. 服务注册（consul.enabled=true 时生效），defer 反注册。
	if err := suite.RegisterService(); err != nil {
		log.Fatalf("register service: %v", err)
	}
	defer func() {
		if err := suite.DeregisterService(); err != nil {
			log.Printf("deregister service: %v", err)
		}
	}()

	// 12. 启动。
	h.Spin()
}
