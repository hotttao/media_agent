package platformtoken

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"media_agent/media_auth/biz/dal"
	domain "media_agent/media_auth/biz/domain/platform_token"
)

func NewPlatformMigrationSource(db *gorm.DB) domain.PlatformMigrationSource {
	return &platformMigrationSource{db: db}
}

func (r *platformMigrationSource) LockPlatformMigration(ctx context.Context) error {
	if !dal.InTransaction(ctx) {
		return errors.New("platform migration requires transaction")
	}
	return dal.FromContext(ctx, r.db).WithContext(ctx).Exec("LOCK TABLE media_auth.service_tokens, media_auth.platform_tokens IN EXCLUSIVE MODE").Error
}

func (r *platformMigrationSource) ReadPlatformMigrationPage(ctx context.Context, after string, limit int) ([]domain.LegacyPlatformRecord, error) {
	if !dal.InTransaction(ctx) || limit < 1 || limit > 100 {
		return nil, errors.New("invalid platform migration page")
	}
	var rows []legacyPlatformRow
	q := dal.FromContext(ctx, r.db).WithContext(ctx).Table("media_auth.service_tokens").Where("is_platform = true")
	if after != "" {
		q = q.Where("id COLLATE \"C\" > ?", after)
	}
	err := q.Order("id COLLATE \"C\" ASC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.LegacyPlatformRecord, 0, len(rows))
	for _, row := range rows {
		t := domain.LegacyPlatformRecord{ID: row.ID, Hash: row.TokenHash, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, RevokedAt: row.RevokedAt}
		if row.CreatedBy != nil {
			t.UserID = *row.CreatedBy
		}
		if row.Token != nil {
			t.Secret = *row.Token
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *platformMigrationSource) DeleteMigratedPlatform(ctx context.Context, id string) error {
	if !dal.InTransaction(ctx) {
		return errors.New("platform migration requires transaction")
	}
	result := dal.FromContext(ctx, r.db).WithContext(ctx).Table("media_auth.service_tokens").Where("id = ? AND is_platform = true", id).Delete(&legacyPlatformRow{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrNotFound
	}
	return nil
}

// Maintenance-only projection: never registered in ordinary CRUD/DI or GORM Gen.
type platformMigrationSource struct{ db *gorm.DB }
type legacyPlatformRow struct {
	ID        string     `gorm:"column:id;primaryKey"`
	TokenHash string     `gorm:"column:token_hash"`
	Token     *string    `gorm:"column:token"`
	CreatedBy *string    `gorm:"column:created_by"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	ExpiresAt *time.Time `gorm:"column:expires_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
}
