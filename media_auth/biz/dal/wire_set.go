// Package dal wire providers。
//
// 与 media_example/biz/dal/wire_set.go 风格一致：ProviderSet 汇总 dal 层构造函数。
// 当前骨架仅占位；§1.3 引入 repo 时在 repo/wire_set.go 注册并聚合到此。
package dal

import "github.com/google/wire"

// ProviderSet 汇总 dal 层构造函数。初始仅含 NewDB；新增 repo 时在 repo/wire_set.go 注册。
var ProviderSet = wire.NewSet(
// NewDB,
// NewTxRunner,
)
