package application

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	d "media_agent/media_auth/biz/domain/application"
	"time"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db} }
func (r *Repo) dbctx(ctx context.Context) *gorm.DB {
	return dal.FromContext(ctx, r.db).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).WithContext(ctx)
}
func convert(v model.Application) d.Application {
	return d.Application{ID: v.ID, Name: v.Name, SecretHash: v.SecretHash, Status: v.Status, CreatedBy: v.CreatedBy, CredentialVersion: v.CredentialVersion, CreatedAt: v.CreatedAt}
}
func mapErr(e error) error {
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return d.ErrNotFound
	}
	if errors.Is(e, gorm.ErrDuplicatedKey) {
		return d.ErrConflict
	}
	return e
}
func (r *Repo) Create(ctx context.Context, a d.Application) (d.Application, error) {
	v := model.Application{ID: a.ID, Name: a.Name, SecretHash: a.SecretHash, Status: a.Status, CreatedBy: a.CreatedBy, CredentialVersion: a.CredentialVersion, CreatedAt: time.Now()}
	if e := r.dbctx(ctx).Create(&v).Error; e != nil {
		return d.Application{}, mapErr(e)
	}
	return convert(v), nil
}
func (r *Repo) List(ctx context.Context) ([]d.Application, error) {
	var rows []model.Application
	if e := r.dbctx(ctx).Order("id").Find(&rows).Error; e != nil {
		return nil, e
	}
	out := make([]d.Application, 0, len(rows))
	for _, v := range rows {
		out = append(out, convert(v))
	}
	return out, nil
}
func (r *Repo) Get(ctx context.Context, id string, lock bool) (d.Application, error) {
	q := r.dbctx(ctx)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var v model.Application
	if e := q.Where("id = ?", id).First(&v).Error; e != nil {
		return d.Application{}, mapErr(e)
	}
	return convert(v), nil
}
func (r *Repo) SetStatus(ctx context.Context, id, status string) (d.Application, error) {
	if e := r.dbctx(ctx).Model(&model.Application{}).Where("id = ?", id).Updates(map[string]any{"status": status, "credential_version": gorm.Expr("credential_version + 1")}).Error; e != nil {
		return d.Application{}, e
	}
	return r.Get(ctx, id, false)
}
func (r *Repo) Rotate(ctx context.Context, id, hash string) (d.Application, error) {
	if e := r.dbctx(ctx).Model(&model.Application{}).Where("id = ?", id).Updates(map[string]any{"secret_hash": hash, "credential_version": gorm.Expr("credential_version + 1")}).Error; e != nil {
		return d.Application{}, e
	}
	return r.Get(ctx, id, false)
}
func (r *Repo) SaveCredential(ctx context.Context, c d.Credential) error {
	db := r.dbctx(ctx)
	if c.Scopes == nil {
		c.Scopes = []string{}
	}
	if err := db.Where("application_id = ? AND expires_at <= CURRENT_TIMESTAMP", c.ApplicationID).Delete(&model.ApplicationCredential{}).Error; err != nil {
		return err
	}
	return db.Create(&model.ApplicationCredential{TokenHash: c.Hash, TeamID: c.TeamID, OrganizationID: c.OrganizationID, Scopes: c.Scopes, ApplicationID: c.ApplicationID, UserID: c.UserID, ExternalID: c.ExternalID, ApplicationVersion: c.ApplicationVersion, AuthVersion: c.AuthVersion, ExpiresAt: c.ExpiresAt}).Error
}

// Read model joins authoritative facts in one database statement; no cached role or mutable permission is trusted.
func (r *Repo) ResolveCredential(ctx context.Context, hash string) (d.Profile, error) {
	var p d.Profile
	result := r.dbctx(ctx).Raw(`SELECT a.id AS application_id,u.id AS user_id,c.external_id,u.display_name,u.email
 FROM media_auth.application_access_credentials c
 JOIN media_auth.applications a ON a.id=c.application_id AND a.status='active' AND a.credential_version=c.application_version
 JOIN media_auth.users u ON u.id=c.user_id AND u.managed_by=a.id AND u.status='active' AND u.auth_version=c.auth_version
 JOIN media_auth.user_identities i ON i.user_id=u.id AND i.provider='app:'||a.id AND i.external_id=c.external_id
 WHERE c.token_hash=? AND c.expires_at>CURRENT_TIMESTAMP`, hash).Scan(&p)
	if result.Error != nil {
		return d.Profile{}, result.Error
	}
	if result.RowsAffected != 1 {
		return d.Profile{}, d.ErrUnauthorized
	}
	return p, nil
}
