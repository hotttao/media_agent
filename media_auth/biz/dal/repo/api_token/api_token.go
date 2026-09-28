package apitoken

import (
	"context"
	"errors"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	domain "media_agent/media_auth/biz/domain/api_token"
	"media_agent/media_auth/biz/shared/ids"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(ctx context.Context, token domain.Token) (domain.Token, error) {
	row := toRow(token)
	if row.ID == "" {
		row.ID = ids.New()
	}
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).Create(&row).Error; err != nil {
		return domain.Token{}, err
	}
	return toDomain(row), nil
}

func (r *Repo) Get(ctx context.Context, id string) (domain.Token, error) {
	var row model.APIToken
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return domain.Token{}, mapNotFound(err)
	}
	return toDomain(row), nil
}

func (r *Repo) GetByTalosKeyID(ctx context.Context, keyID string) (domain.Token, error) {
	var row model.APIToken
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).Where("talos_key_id = ?", keyID).First(&row).Error; err != nil {
		return domain.Token{}, mapNotFound(err)
	}
	return toDomain(row), nil
}

func (r *Repo) ListBySubject(ctx context.Context, userID string) ([]domain.Token, error) {
	var rows []model.APIToken
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).Where("subject_user_id = ?", userID).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Token, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomain(row))
	}
	return out, nil
}

func (r *Repo) MarkRevoking(ctx context.Context, id, subjectUserID string) (domain.Token, error) {
	db := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.APIToken{}).
		Where("id = ? AND subject_user_id = ? AND status = ?", id, subjectUserID, domain.StatusActive).
		Update("status", domain.StatusRevoking)
	if db.Error != nil {
		return domain.Token{}, db.Error
	}
	if db.RowsAffected == 0 {
		return domain.Token{}, domain.ErrNotFound
	}
	return r.Get(ctx, id)
}

func (r *Repo) MarkRevoked(ctx context.Context, id string, at time.Time) error {
	db := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.APIToken{}).Where("id = ?", id).
		Updates(map[string]any{"status": domain.StatusRevoked, "revoked_at": at})
	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repo) ReplaceTalosKey(ctx context.Context, id, keyID string, expiresAt *time.Time, at time.Time) (domain.Token, error) {
	db := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.APIToken{}).Where("id = ?", id).
		Updates(map[string]any{"talos_key_id": keyID, "expires_at": expiresAt, "rotated_at": at, "status": domain.StatusActive})
	if db.Error != nil {
		return domain.Token{}, db.Error
	}
	if db.RowsAffected == 0 {
		return domain.Token{}, domain.ErrNotFound
	}
	return r.Get(ctx, id)
}

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	return err
}

func toRow(token domain.Token) model.APIToken {
	row := model.APIToken{
		ID: token.ID, TalosKeyID: token.TalosKeyID, ActorType: string(token.ActorType), ActorID: token.ActorID,
		Name: token.Name, Scopes: pq.StringArray(append([]string(nil), token.Scopes...)), Status: string(token.Status),
		ExpiresAt: token.ExpiresAt, CreatedBy: token.CreatedBy, CreatedAt: token.CreatedAt,
		RotatedAt: token.RotatedAt, RevokedAt: token.RevokedAt,
	}
	if token.SubjectUserID != "" {
		row.SubjectUserID = &token.SubjectUserID
	}
	if token.OrganizationID != "" {
		row.OrganizationID = &token.OrganizationID
	}
	return row
}

func toDomain(row model.APIToken) domain.Token {
	token := domain.Token{
		ID: row.ID, TalosKeyID: row.TalosKeyID, ActorType: domain.ActorType(row.ActorType), ActorID: row.ActorID,
		Name: row.Name, Scopes: append([]string(nil), row.Scopes...), Status: domain.Status(row.Status), ExpiresAt: row.ExpiresAt,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, RotatedAt: row.RotatedAt, RevokedAt: row.RevokedAt,
	}
	if row.SubjectUserID != nil {
		token.SubjectUserID = *row.SubjectUserID
	}
	if row.OrganizationID != nil {
		token.OrganizationID = *row.OrganizationID
	}
	return token
}
