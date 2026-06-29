// init.go：media_example 客户端初始化唯一入口。
//
// 按客户端调用指南（contributing/client.md）§7.2：
//  1. 调用 clienthertz.NewClientSuite(loader) 构造基底（收 loader 不收 cfg，
//     可热更新能力组件在 suite 内部按需 loader.Subscribe）；
//  2. 对每个下游 app 调用 baseSuite.NewAppClientSuite(<app>) + init<App>Clients；
//  3. 下游 app 配置缺失必须返回错误。
//
// 当前模板无下游，Init 仅完成 baseSuite 构造，保留入口骨架。
package client

import (
	"fmt"

	"media_agent/hertz_infra/clienthertz"
	"media_agent/hertz_infra/config"
)

// Init 是当前服务客户端初始化唯一入口，由 main.go 调用。
//
// loader：配置 Loader（收 loader 不收 cfg，对称 serverhertz）。
// 返回 baseSuite 供后续（若有下游）按 app 拆分使用。
func Init(loader *config.Loader) (*clienthertz.ClientSuite, error) {
	if loader == nil {
		return nil, fmt.Errorf("client: nil loader")
	}

	// 进程级基底（consul_discovery / otel / prometheus），收 loader。
	baseSuite := clienthertz.NewClientSuite(loader)

	// 此处注册各下游 app 的初始化函数，例：
	//   appSuite, err := baseSuite.NewAppClientSuite("media_scheduler")
	//   if err != nil {
	//       return nil, fmt.Errorf("client: init media_scheduler: %w", err)
	//   }
	//   shared, _ := appSuite.NewClient()                          // 同一 app 多 service 共享一个 client
	//   _ = shared                                                  // 传给各 typed client: xxx.WithHertzClient(shared)

	return baseSuite, nil
}
