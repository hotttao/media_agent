package environment_grant

import (
	"context"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	d "media_agent/media_control/biz/domain/environment_grant"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) EnvironmentExists(ctx context.Context, id string) bool {
	var n int64
	dal.FromContext(ctx, r.db).Model(&model.RuntimeEnvironment{}).Where("id=?", id).Count(&n)
	return n > 0
}
func (r *Repository) Create(ctx context.Context, v *d.Grant) error {
	return dal.FromContext(ctx, r.db).Create(&model.EnvironmentGrant{ID: v.ID, EnvironmentID: v.EnvironmentID, SubjectID: v.SubjectID, SubjectType: v.SubjectType, Permission: v.Permission, Status: v.Status, CreatedAt: v.CreatedAt, RevokedAt: v.RevokedAt}).Error
}
func (r *Repository) List(ctx context.Context, id string) (out []d.Grant, err error) {
	var rows []model.EnvironmentGrant
	err = dal.FromContext(ctx, r.db).Where("environment_id=?", id).Order("created_at DESC").Find(&rows).Error
	for _, v := range rows {
		out = append(out, toGrant(v))
	}
	return
}
func (r *Repository) Get(ctx context.Context, id, env string) (*d.Grant, error) {
	var v model.EnvironmentGrant
	if err := dal.FromContext(ctx, r.db).Where("id=? AND environment_id=?", id, env).First(&v).Error; err != nil {
		return nil, d.ErrNotFound
	}
	x := toGrant(v)
	return &x, nil
}
func (r *Repository) Revoke(ctx context.Context, v *d.Grant) error {
	res := dal.FromContext(ctx, r.db).Model(&model.EnvironmentGrant{}).Where("id=? AND status=?", v.ID, "active").Updates(map[string]any{"status": "revoked", "revoked_at": v.RevokedAt})
	if res.RowsAffected == 0 {
		return d.ErrRevoked
	}
	return res.Error
}
func toGrant(v model.EnvironmentGrant) d.Grant {
	return d.Grant{ID: v.ID, EnvironmentID: v.EnvironmentID, SubjectID: v.SubjectID, SubjectType: v.SubjectType, Permission: v.Permission, Status: v.Status, CreatedAt: v.CreatedAt, RevokedAt: v.RevokedAt}
}
