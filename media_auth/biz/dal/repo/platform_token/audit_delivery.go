package platformtoken

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	domain "media_agent/media_auth/biz/domain/platform_token"
)

func (r *Repo) ClaimAudit(ctx context.Context, lease string, now, until time.Time, limit int) ([]domain.AuditDelivery, error) {
	if !dal.InTransaction(ctx) || !domain.ValidIdentity(lease) || now.IsZero() || !until.After(now) || until.Sub(now) > 5*time.Minute || limit < 1 || limit > 100 {
		return nil, domain.ErrInvalid
	}
	db := dal.FromContext(ctx, r.db).WithContext(ctx)
	var rows []model.PlatformTokenAudit
	if err := db.Where("delivered_at IS NULL AND next_attempt_at <= ? AND (lease_until IS NULL OR lease_until <= ?)", now, now).
		Order("next_attempt_at ASC, occurred_at ASC, id ASC").Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.AuditDelivery, 0, len(rows))
	for _, row := range rows {
		event := domain.AuditEvent{ID: row.ID, UserID: row.UserID, CredentialID: row.CredentialID, Operation: row.Operation, CallerService: row.CallerService, Purpose: row.Purpose, OccurredAt: row.OccurredAt}
		if !event.Valid() {
			return nil, domain.ErrInvalid
		}
		update := db.Model(&model.PlatformTokenAudit{}).Where("id=? AND delivered_at IS NULL", row.ID).
			Updates(map[string]any{"lease_id": lease, "lease_until": until, "attempts": gorm.Expr("LEAST(attempts, 2147483646) + 1")})
		if update.Error != nil {
			return nil, update.Error
		}
		if update.RowsAffected != 1 {
			return nil, domain.ErrConflict
		}
		attempts := row.Attempts
		if attempts < 2147483647 {
			attempts++
		}
		result = append(result, domain.AuditDelivery{Event: event, LeaseID: lease, LeaseUntil: until, Attempts: attempts})
	}
	return result, nil
}

func (r *Repo) ConfirmAudit(ctx context.Context, id, lease string, now time.Time) (bool, error) {
	if !dal.InTransaction(ctx) || !domain.ValidIdentity(id) || !domain.ValidIdentity(lease) || now.IsZero() {
		return false, domain.ErrInvalid
	}
	result := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.PlatformTokenAudit{}).
		Where("id=? AND lease_id=? AND delivered_at IS NULL AND lease_until > GREATEST(?::timestamptz, clock_timestamp())", id, lease, now).
		Updates(map[string]any{"delivered_at": now, "lease_id": nil, "lease_until": nil})
	return result.RowsAffected == 1, result.Error
}

func (r *Repo) RetryAudit(ctx context.Context, id, lease string, now, next time.Time) (bool, error) {
	if !dal.InTransaction(ctx) || !domain.ValidIdentity(id) || !domain.ValidIdentity(lease) || now.IsZero() || !next.After(now) {
		return false, domain.ErrInvalid
	}
	result := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.PlatformTokenAudit{}).
		Where("id=? AND lease_id=? AND delivered_at IS NULL AND lease_until > GREATEST(?::timestamptz, clock_timestamp())", id, lease, now).
		Updates(map[string]any{"lease_id": nil, "lease_until": nil, "next_attempt_at": next})
	return result.RowsAffected == 1, result.Error
}

var _ domain.AuditOutbox = (*Repo)(nil)
