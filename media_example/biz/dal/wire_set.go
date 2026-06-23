// Package dal 存放数据库访问：db.go（初始化）、transaction.go（事务抽象）、model/（GORM row model）、
// query/（GORM Gen 生成）、repo/（domain 仓储实现）。
//
// 禁止 import biz/handler、hertz_gen/model；不得把 GORM row model 返回给 service/workflow。
// service/workflow/policy/domain 不得 import biz/dal/query 或 biz/dal/model。
package dal

import "github.com/google/wire"

// ProviderSet 汇总 dal 层构造函数。初始仅含 NewDB；新增 repo 时在 repo/wire_set.go 注册。
var ProviderSet = wire.NewSet(
// NewDB,
)
