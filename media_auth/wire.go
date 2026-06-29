//go:build wireinject
// +build wireinject

// wire.go 是 wire 注入模板，仅在 `wire` CLI 生成代码时启用。
//
// 真实运行时（go build / go run .）由 wire_gen.go 提供 InitDI() 实现。
//
// 用法（当 NewService 签名变化后重新生成 wire_gen.go）：
//  1. go install github.com/google/wire/cmd/wire@latest
//  2. cd media_auth && wire
//
// 当前骨架无 service 实现；后续 §1.4 引入 repo/service/workflow 依赖时，
// 在 wire.Build(...) 里追加对应 ProviderSet。
package main

import (
	"github.com/google/wire"
)

// InitDI 全局依赖注入入口。
func InitDI() {
	wire.Build()
	return nil
}
