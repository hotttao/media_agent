package platformtoken

import (
	"context"
	"errors"

	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	domain "media_agent/media_auth/biz/domain/platform_token"
)

var errReadiness = errors.New("platform custody prerequisites unavailable")

func (r *Repo) CustodyMigrationReady(ctx context.Context) (bool, error) {
	db := dal.FromContext(ctx, r.db).WithContext(ctx)
	var migration struct {
		Version int64
		Dirty   bool
	}
	if err := db.Raw("SELECT version, dirty FROM media_auth.schema_migrations").Scan(&migration).Error; err != nil {
		return false, errReadiness
	}
	if migration.Version < 16 || migration.Dirty {
		return false, nil
	}
	var old bool
	if err := db.Raw("SELECT EXISTS (SELECT 1 FROM media_auth.service_tokens WHERE is_platform = true)").Scan(&old).Error; err != nil {
		return false, errReadiness
	}
	if old {
		return false, nil
	}
	// Probe both targets, including audit, even when there are no credentials.
	if err := db.Raw("SELECT 1 FROM media_auth.platform_token_audit_outbox LIMIT 1").Scan(new(int)).Error; err != nil {
		return false, errReadiness
	}
	return true, nil
}

func (r *Repo) ReadCustodyPage(ctx context.Context, after string, limit int) ([]domain.Token, error) {
	if limit < 1 || limit > 100 || after != "" && !domain.ValidIdentity(after) {
		return nil, errReadiness
	}
	q := dal.FromContext(ctx, r.db).WithContext(ctx).Order("id COLLATE \"C\"").Limit(limit)
	if after != "" {
		q = q.Where("id COLLATE \"C\" > ?", after)
	}
	var rows []model.PlatformToken
	if err := q.Find(&rows).Error; err != nil {
		return nil, errReadiness
	}
	out := make([]domain.Token, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomain(row))
	}
	return out, nil
}
