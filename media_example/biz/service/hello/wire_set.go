// Package hello — wire providers。
//
// 与 media-materials/biz/service/<domain>/wire_set.go 风格一致：
// 每个 service domain 自带一个 ProviderSet，被 media_example/wire.go 聚合使用。
package hello

import "github.com/google/wire"

// ProviderSet 暴露 hello 用例服务的构造函数。
// 当前 NewService 无依赖；后续若引入 dal/repo 依赖，改 NewService 签名即可，
// 无需调整 wire.go 的 Build 列表。
var ProviderSet = wire.NewSet(NewService)
