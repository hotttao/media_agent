// transaction.go：事务抽象（TxRunner + ctx 传递 tx）。
//
// 事务边界由 TxRunner 统一开启，通过 context.Context 隐式传递 tx，*gorm.DB 不泄漏给上层。
// 事务开启点（architecture.md「事务 TxRunner + ctx 传递」）：
//   - 单 domain 用例：service 顶部 RunInTx；
//   - 跨 domain 用例：workflow 顶部 RunInTx；
//   - service 内部方法 / repo：不开事务，只透传 ctx，用 FromContext(ctx, db) 取连接。
//
// repo 永远 FromContext(ctx, db)：ctx 带 tx 用 tx（加入上层事务），否则用默认 db（非事务读）。
// 这样无论上层是否在事务内，repo 行为一致。driver 无关（postgres/mysql 通用）。
package dal

import (
	"context"

	"gorm.io/gorm"
)

// TxRunner 在一个 DB 事务内执行 fn；tx 经 ctx 向下传递。
// service/workflow 依赖此接口，不直接依赖 *gorm.DB。
type TxRunner interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// txRunner 是 TxRunner 的默认实现，持有 *gorm.DB。
type txRunner struct {
	db *gorm.DB
}

// NewTxRunner 基于 *gorm.DB 构造 TxRunner。db 为 nil 时返回 nil（database 禁用场景）。
func NewTxRunner(db *gorm.DB) TxRunner {
	if db == nil {
		return nil
	}
	return &txRunner{db: db}
}

// RunInTx 在一个事务内执行 fn：fn 返回 error 回滚，nil 提交。
// 把 tx 塞进 ctx（WithTx），下游 repo 经 FromContext 取 tx。
func (r *txRunner) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(WithTx(ctx, tx))
	})
}

// ctxKey 是 ctx 中携带 *gorm.DB（事务 tx）的 key 类型，避免导出冲突。
type ctxKey struct{}

// WithTx 把事务 tx 关联到 ctx，返回新 ctx。供 TxRunner.RunInTx 内部使用。
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, ctxKey{}, tx)
}

// FromContext 取连接：ctx 带 tx（上层在事务内）则用 tx，否则用默认 db（非事务）。
// repo 唯一获取 *gorm.DB 的入口——禁止 repo 自行开事务。
func FromContext(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(ctxKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return db
}
