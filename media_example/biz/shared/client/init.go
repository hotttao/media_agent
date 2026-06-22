// init.go：media_example 客户端初始化唯一入口。
//
// 按客户端调用指南（contributing/client-cn.md）§7.2：
//   1. 调用 clienthertz 的进程级初始化（tracing / metrics 若未在 main 调用）；
//   2. 调用 clienthertz.BuildBaseSuite 构造基底；
//   3. 对每个下游 app 调用 init<App>Clients(cfg, baseSuite)；
//   4. 下游 app 配置缺失必须返回错误。
//
// 当前模板无下游，Init 仅完成 baseSuite 构造，保留入口骨架。
package client

import (
	"fmt"

	"media_agent/hertz_gen/config"
	"media_agent/hertz_infra/clienthertz"
)

// Init 是当前服务客户端初始化唯一入口，由 main.go 调用。
//
// cfg：完整配置树。
// 返回 baseSuite 供后续（若有下游）按 app 拆分使用。
func Init(cfg *config.Config) (*clienthertz.ClientSuite, error) {
	if cfg == nil {
		return nil, fmt.Errorf("client: nil config")
	}

	// 进程级基底（consul_discovery / otel / prometheus）。
	baseSuite := clienthertz.BuildBaseSuite(cfg)

	// 此处注册各下游 app 的初始化函数，例：
	//   if err := initMediaSchedulerClients(cfg, baseSuite); err != nil {
	//       return nil, fmt.Errorf("client: init media_scheduler: %w", err)
	//   }

	return baseSuite, nil
}
