package auth

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
)

// PlatformUserActive supplies only the current user fact. Locking requires the
// caller's transaction; updates to users.status serialize with this row lock.
func (r *repo) PlatformUserActive(ctx context.Context, id string, lock bool) (bool, error) {
	if lock && !dal.InTransaction(ctx) {
		return false, errors.New("platform user lock requires transaction")
	}
	q := dal.FromContext(ctx, r.db).WithContext(ctx).Select("id", "status").Where("id = ?", id)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row model.User
	err := q.Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return row.Status == "active", nil
}

// NewPlatformUserReader exposes the narrow query/lock port without changing
// every legacy auth.Repository test double.
func NewPlatformUserReader(db *gorm.DB) interface {
	PlatformUserActive(context.Context, string, bool) (bool, error)
} {
	return &repo{db: db}
}
