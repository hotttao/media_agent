// main.go 是 Hertz 程序入口。
//
// 按服务端治理指南（contributing/server-cn.md）§8：
//  1. config.Load 加载配置；
//  2. serverhertz.NewServerSuite 构造 ServerSuite；
//  3. serverhertz.InitTracing / InitMetrics 进程级初始化（TODO：待治理实现补齐）；
//  4. serverhertz.NewServer 创建 server + 挂全局 middleware；
//  5. 注册生成的业务路由（hz update 后由 register(h) 提供）；
//  6. serverhertz.RegisterService 注册服务，defer 反注册；
//  7. h.Spin。
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

	// 5. 初始化下游 client 装配层（当前模板无下游，baseSuite 仅做进程级装配）。
	if _, err := client.Init(loader.Current()); err != nil {
		log.Fatalf("init clients: %v", err)
	}

	// 6. 创建 Hertz server + 挂全局 middleware。
	h := serverhertz.NewServer(suite)

	// 7. 注册生成的业务路由（hz update 后恢复 register(h) 调用）。
	// register(h)

	// 8. 服务注册（consul.enabled=true 时生效），defer 反注册。
	ctx := context.Background()
	if err := suite.RegisterService(); err != nil {
		log.Fatalf("register service: %v", err)
	}
	defer func() {
		if err := suite.DeregisterService(); err != nil {
			log.Printf("deregister service: %v", err)
		}
	}()
	_ = ctx

	// 9. 启动。
	h.Spin()
}
