package dal

import (
	"context"

	"gorm.io/gorm"
)

type TxRunner interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type txRunner struct{ db *gorm.DB }

func NewTxRunner(db *gorm.DB) TxRunner {
	if db == nil {
		return nil
	}
	return &txRunner{db: db}
}

func (r *txRunner) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, ctxKey{}, tx))
	})
}

type ctxKey struct{}

func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, ctxKey{}, tx)
}

func FromContext(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(ctxKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return db
}
