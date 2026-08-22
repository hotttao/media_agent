package dal

import (
	"context"
	"gorm.io/gorm"
)

type TxRunner interface {
	RunInTx(context.Context, func(context.Context) error) error
}
type txRunner struct{ db *gorm.DB }
type txKey struct{}

func NewTxRunner(db *gorm.DB) TxRunner { return &txRunner{db: db} }
func (r *txRunner) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(context.WithValue(ctx, txKey{}, tx)) })
}
func FromContext(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return db.WithContext(ctx)
}
