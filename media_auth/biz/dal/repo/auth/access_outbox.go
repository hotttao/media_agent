package auth

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	domainauth "media_agent/media_auth/biz/domain/auth"
)

type accessOutboxRepo struct{ db *gorm.DB }

func NewAccessRevocationOutbox(db *gorm.DB) domainauth.AccessRevocationOutbox {
	return &accessOutboxRepo{db: db}
}

func (r *accessOutboxRepo) Enqueue(ctx context.Context, event domainauth.AccessRevocationEvent) error {
	now := time.Now()
	row := model.UserAccessOutbox{
		ID: event.ID, UserID: event.UserID, AuthVersion: event.AuthVersion,
		EventType: "UserAccessRevoked", Status: "pending", AvailableAt: now,
		CreatedAt: now, UpdatedAt: now,
	}
	return dal.FromContext(ctx, r.db).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

func (r *accessOutboxRepo) ClaimPending(ctx context.Context, limit int) ([]domainauth.AccessRevocationEvent, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows []model.UserAccessOutbox
	err := dal.FromContext(ctx, r.db).
		Where("status = 'pending' AND available_at <= now()").
		Order("created_at ASC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domainauth.AccessRevocationEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, domainauth.AccessRevocationEvent{ID: row.ID, UserID: row.UserID, AuthVersion: row.AuthVersion, Attempts: row.Attempts})
	}
	return out, nil
}

func (r *accessOutboxRepo) MarkDelivered(ctx context.Context, id string) error {
	now := time.Now()
	return dal.FromContext(ctx, r.db).Model(&model.UserAccessOutbox{}).Where("id = ? AND status = 'pending'", id).
		Updates(map[string]any{"status": "delivered", "delivered_at": now, "updated_at": now, "last_error": nil}).Error
}

func (r *accessOutboxRepo) MarkFailed(ctx context.Context, id, message string) error {
	return dal.FromContext(ctx, r.db).Model(&model.UserAccessOutbox{}).Where("id = ? AND status = 'pending'", id).
		Updates(map[string]any{
			"attempts": gorm.Expr("attempts + 1"), "last_error": message,
			"available_at": gorm.Expr("now() + interval '1 second' * LEAST(60, power(2, LEAST(attempts, 6)))"),
			"updated_at":   time.Now(),
		}).Error
}
