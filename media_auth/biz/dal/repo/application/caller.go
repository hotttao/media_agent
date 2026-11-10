package application

import (
	"context"
	"gorm.io/gorm/clause"
	"media_agent/media_auth/biz/dal/model"
	d "media_agent/media_auth/biz/domain/application"
)

func (r *Repo) PutTarget(ctx context.Context, t d.MCPTarget) error {
	v := model.ApplicationMCPTarget{DefinitionID: t.DefinitionID, ApplicationID: t.ApplicationID, Origin: t.Origin}
	q := r.dbctx(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "definition_id"}}, DoUpdates: clause.AssignmentColumns([]string{"origin"}), Where: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "application_mcp_targets.application_id = ?", Vars: []any{t.ApplicationID}}}}}).Create(&v)
	if q.Error != nil {
		return mapErr(q.Error)
	}
	if q.RowsAffected != 1 {
		return d.ErrConflict
	}
	return nil
}
func (r *Repo) DeleteTarget(ctx context.Context, app, id string) error {
	return r.dbctx(ctx).Where("application_id=? AND definition_id=?", app, id).Delete(&model.ApplicationMCPTarget{}).Error
}
func (r *Repo) Target(ctx context.Context, id string) (d.MCPTarget, error) {
	var v model.ApplicationMCPTarget
	e := r.dbctx(ctx).Where("definition_id=?", id).Take(&v).Error
	return d.MCPTarget{ApplicationID: v.ApplicationID, DefinitionID: v.DefinitionID, Origin: v.Origin}, mapErr(e)
}
func (r *Repo) CallerProfile(ctx context.Context, app, user string) (d.Profile, error) {
	var p d.Profile
	q := r.dbctx(ctx).Raw(`SELECT a.id AS application_id,u.id AS user_id,i.external_id,u.display_name,u.email FROM media_auth.applications a JOIN media_auth.users u ON u.managed_by=a.id AND u.status='active' JOIN media_auth.user_identities i ON i.user_id=u.id AND i.provider='app:'||a.id WHERE a.id=? AND a.status='active' AND u.id=?`, app, user).Scan(&p)
	if q.Error != nil {
		return p, q.Error
	}
	if q.RowsAffected != 1 {
		return p, d.ErrForbidden
	}
	return p, nil
}
