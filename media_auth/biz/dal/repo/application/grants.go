package application

import (
	"context"
	"github.com/lib/pq"
	"gorm.io/gorm/clause"
	"media_agent/media_auth/biz/dal/model"
	d "media_agent/media_auth/biz/domain/application"
	"time"
)

func (r *Repo) TeamTarget(ctx context.Context, id string) (string, bool, error) {
	var v struct {
		OrganizationID string
		Active         bool
	}
	q := r.dbctx(ctx).Raw(`SELECT t.organization_id,(t.status='active' AND o.status='active') AS active FROM media_auth.teams t JOIN media_auth.organizations o ON o.id=t.organization_id WHERE t.id=?`, id).Scan(&v)
	if q.Error != nil {
		return "", false, q.Error
	}
	if q.RowsAffected != 1 {
		return "", false, d.ErrNotFound
	}
	return v.OrganizationID, v.Active, nil
}
func (r *Repo) PutGrant(ctx context.Context, g d.TeamGrant) error {
	return r.dbctx(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "application_id"}, {Name: "team_id"}}, DoUpdates: clause.AssignmentColumns([]string{"organization_id", "scopes"})}).Create(&model.ApplicationTeamGrant{ApplicationID: g.ApplicationID, TeamID: g.TeamID, OrganizationID: g.OrganizationID, Scopes: g.Scopes}).Error
}
func (r *Repo) DeleteGrant(ctx context.Context, app, team string) error {
	return r.dbctx(ctx).Where("application_id=? AND team_id=?", app, team).Delete(&model.ApplicationTeamGrant{}).Error
}
func (r *Repo) Grant(ctx context.Context, app, team string) (d.TeamGrant, error) {
	var v model.ApplicationTeamGrant
	if e := r.dbctx(ctx).Where("application_id=? AND team_id=?", app, team).Take(&v).Error; e != nil {
		return d.TeamGrant{}, mapErr(e)
	}
	return d.TeamGrant{ApplicationID: v.ApplicationID, TeamID: v.TeamID, OrganizationID: v.OrganizationID, Scopes: v.Scopes}, nil
}
func (r *Repo) ResolveBusinessCredential(ctx context.Context, hash string) (d.BusinessCredential, error) {
	var v struct {
		ApplicationID, UserID, ExternalID, DisplayName, Email, TeamID, OrganizationID string
		Scopes                                                                        pq.StringArray `gorm:"type:text[]"`
		ExpiresAt                                                                     time.Time
	}
	q := r.dbctx(ctx).Raw(`SELECT a.id AS application_id,u.id AS user_id,c.external_id,u.display_name,u.email,c.team_id,c.organization_id,c.scopes,c.expires_at
 FROM media_auth.application_access_credentials c JOIN media_auth.applications a ON a.id=c.application_id AND a.status='active' AND a.credential_version=c.application_version
 JOIN media_auth.users u ON u.id=c.user_id AND u.managed_by=a.id AND u.status='active' AND u.auth_version=c.auth_version
 JOIN media_auth.user_identities i ON i.user_id=u.id AND i.provider='app:'||a.id AND i.external_id=c.external_id
 JOIN media_auth.application_team_grants g ON g.application_id=a.id AND g.team_id=c.team_id AND g.organization_id=c.organization_id AND c.scopes <@ g.scopes
 JOIN media_auth.teams t ON t.id=g.team_id AND t.organization_id=g.organization_id AND t.status='active'
 JOIN media_auth.organizations o ON o.id=g.organization_id AND o.status='active'
 JOIN media_auth.team_members tm ON tm.team_id=t.id AND tm.user_id=u.id
 JOIN media_auth.organization_members om ON om.organization_id=o.id AND om.user_id=u.id
 WHERE c.token_hash=? AND c.expires_at>CURRENT_TIMESTAMP AND c.team_id<>''`, hash).Scan(&v)
	if q.Error != nil {
		return d.BusinessCredential{}, q.Error
	}
	if q.RowsAffected != 1 {
		return d.BusinessCredential{}, d.ErrUnauthorized
	}
	return d.BusinessCredential{Profile: d.Profile{ApplicationID: v.ApplicationID, UserID: v.UserID, ExternalID: v.ExternalID, DisplayName: v.DisplayName, Email: v.Email}, TeamID: v.TeamID, OrganizationID: v.OrganizationID, Scopes: v.Scopes, ExpiresAt: v.ExpiresAt}, nil
}
