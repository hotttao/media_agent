package auth

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"media_agent/media_auth/biz/dal/model"
	d "media_agent/media_auth/biz/domain/auth"
	"media_agent/media_auth/biz/shared/ids"
	"time"
)

func NewManagedRepo(db *gorm.DB) d.ManagedUserRepo { return &repo{db} }
func (r *repo) CreateManaged(ctx context.Context, applicationID, name, email string) (d.User, error) {
	id := ids.New()
	v := model.User{ID: id, Username: "app-user:" + id, ManagedBy: &applicationID, DisplayName: name, Email: strPtr(email), Role: "user", Status: "active", AuthVersion: 1, CreatedAt: time.Now()}
	if e := r.q(ctx).UnderlyingDB().WithContext(ctx).Create(&v).Error; e != nil {
		return d.User{}, e
	}
	return rowToDomain(&v), nil
}
func (r *repo) LockManaged(ctx context.Context, id string) (d.User, error) {
	var v model.User
	e := r.q(ctx).UnderlyingDB().WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&v).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return d.User{}, d.ErrUserNotFound
	}
	return rowToDomain(&v), e
}
func (r *repo) UpdateManagedProfile(ctx context.Context, id, name, email string) error {
	return r.q(ctx).UnderlyingDB().WithContext(ctx).Model(&model.User{}).Where("id = ? AND managed_by IS NOT NULL AND status='active'", id).Updates(map[string]any{"display_name": name, "email": strPtr(email)}).Error
}
