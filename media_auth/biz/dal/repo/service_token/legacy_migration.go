package servicetoken

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	domain "media_agent/media_auth/biz/domain/service_token"
)

func NewPlatformMigrationSource(db *gorm.DB) domain.PlatformMigrationSource { return &repo{db: db} }

func (r *repo) LockPlatformMigration(ctx context.Context) error {
	if !dal.InTransaction(ctx) {
		return errors.New("platform migration requires transaction")
	}
	return dal.FromContext(ctx, r.db).WithContext(ctx).Exec("LOCK TABLE media_auth.service_tokens, media_auth.platform_tokens IN EXCLUSIVE MODE").Error
}

func (r *repo) ReadPlatformMigrationPage(ctx context.Context, after string, limit int) ([]domain.LegacyPlatformRecord, error) {
	if !dal.InTransaction(ctx) || limit < 1 || limit > 100 {
		return nil, errors.New("invalid platform migration page")
	}
	var rows []model.ServiceToken
	q := dal.FromContext(ctx, r.db).WithContext(ctx).Where("is_platform = true")
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

func (r *repo) DeleteMigratedPlatform(ctx context.Context, id string) error {
	if !dal.InTransaction(ctx) {
		return errors.New("platform migration requires transaction")
	}
	result := dal.FromContext(ctx, r.db).WithContext(ctx).Where("id = ? AND is_platform = true", id).Delete(&model.ServiceToken{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrNotFound
	}
	return nil
}
