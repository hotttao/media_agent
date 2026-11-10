package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	d "media_agent/media_control/biz/domain/workspace"
)

func (r *Repository) LockApplicationKey(ctx context.Context, b d.ApplicationBinding) error {
	key, _ := json.Marshal([]string{b.ApplicationID, b.UserID, b.TeamID, b.Name})
	return dal.FromContext(ctx, r.db).WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", string(key)).Error
}
func (r *Repository) FindApplicationBinding(ctx context.Context, b d.ApplicationBinding) (d.ApplicationBinding, error) {
	var v model.ApplicationBinding
	e := dal.FromContext(ctx, r.db).WithContext(ctx).Where("application_id=? AND user_id=? AND team_id=? AND name=?", b.ApplicationID, b.UserID, b.TeamID, b.Name).Take(&v).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return d.ApplicationBinding{}, d.ErrNotFound
	}
	return d.ApplicationBinding(v), e
}
func (r *Repository) FindApplicationWorkspace(ctx context.Context, b d.ApplicationBinding) (d.ApplicationBinding, error) {
	var v model.ApplicationBinding
	e := dal.FromContext(ctx, r.db).WithContext(ctx).Where("application_id=? AND user_id=? AND team_id=? AND workspace_id=?", b.ApplicationID, b.UserID, b.TeamID, b.WorkspaceID).Take(&v).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return d.ApplicationBinding{}, d.ErrNotFound
	}
	return d.ApplicationBinding(v), e
}
func (r *Repository) CreateApplicationBinding(ctx context.Context, b d.ApplicationBinding) error {
	v := model.ApplicationBinding(b)
	return dal.FromContext(ctx, r.db).WithContext(ctx).Create(&v).Error
}
