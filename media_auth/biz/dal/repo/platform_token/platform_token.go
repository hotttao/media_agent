package platformtoken

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	domain "media_agent/media_auth/biz/domain/platform_token"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(ctx context.Context, t domain.Token) error {
	if !t.ValidStorage() {
		return domain.ErrInvalid
	}
	row := model.PlatformToken{ID: t.ID, UserID: t.UserID, TokenHash: t.TokenHash, Ciphertext: t.Ciphertext, Nonce: t.Nonce, KeyVersion: t.KeyVersion, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt}
	err := dal.FromContext(ctx, r.db).WithContext(ctx).Create(&row).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return domain.ErrConflict
	}
	var sqlErr interface{ SQLState() string }
	if errors.As(err, &sqlErr) && sqlErr.SQLState() == "23505" {
		return domain.ErrConflict
	}
	return err
}

func (r *Repo) GetCurrent(ctx context.Context, user string) (domain.Token, error) {
	if !domain.ValidIdentity(user) {
		return domain.Token{}, domain.ErrInvalid
	}
	var row model.PlatformToken
	err := dal.FromContext(ctx, r.db).WithContext(ctx).Where("user_id = ? AND revoked_at IS NULL", user).First(&row).Error
	return toDomain(row), mapNotFound(err)
}

func (r *Repo) GetByHash(ctx context.Context, hash []byte) (domain.Token, error) {
	if len(hash) != 32 {
		return domain.Token{}, domain.ErrInvalid
	}
	var row model.PlatformToken
	err := dal.FromContext(ctx, r.db).WithContext(ctx).Where("token_hash = ? AND revoked_at IS NULL", hash).First(&row).Error
	return toDomain(row), mapNotFound(err)
}

func (r *Repo) RevokeForUser(ctx context.Context, user string, at time.Time) error {
	if !domain.ValidIdentity(user) || at.IsZero() {
		return domain.ErrInvalid
	}
	return dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.PlatformToken{}).Where("user_id = ? AND revoked_at IS NULL", user).Update("revoked_at", at).Error
}

func toDomain(t model.PlatformToken) domain.Token {
	return domain.Token{ID: t.ID, UserID: t.UserID, TokenHash: append([]byte(nil), t.TokenHash...), Ciphertext: append([]byte(nil), t.Ciphertext...), Nonce: append([]byte(nil), t.Nonce...), KeyVersion: t.KeyVersion, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt}
}
func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	return err
}

var _ domain.Repository = (*Repo)(nil)
