//go:build wireinject
// +build wireinject

// wire.go 是 wire 注入模板，仅在 `wire` CLI 生成代码时启用。
//
// 真实运行时（go build / go run .）由 wire_gen.go 提供 InitDI() 实现。
//
// 用法（当 NewService 签名变化后重新生成 wire_gen.go）：
//  1. go install github.com/google/wire/cmd/wire@latest
//  2. cd media_example && wire
//
// 当前 NewService() 无依赖，所以模板非常简单；后续若引入 repo 依赖，
// 在 wire.Build(...) 里追加对应 ProviderSet 即可。
package main

import (
	"github.com/google/wire"

	hello_handler "media_agent/media_example/biz/handler/hello"
	hello_service "media_agent/media_example/biz/service/hello"
)

// DIResult 是 wire 注入后的产物集合。当前仅含 hello service（单域）。
// 字段名必须与 wire.Struct(new(DIResult), "*") 解析的导出字段一一对应。
type DIResult struct {
	HelloSvc *hello_service.Service
}

// InitDI 全局依赖注入入口。
//
// 2026-06-23：参照 media-materials 的 DIResult 模式；本地无 wire CLI，
// 由 wire_gen.go 手写等价实现，避免函数字面量在 wire CLI 不支持的限制。
func InitDI() (*DIResult, error) {
	wire.Build(
		hello_service.ProviderSet,
		// 显式列出 handler 包级变量，便于 InitDI 后续回填。
		hello_handler.HelloSvc,
		wire.Struct(new(DIResult), "*"),
	)
	return nil, nil
}
