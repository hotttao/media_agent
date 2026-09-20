package authorizationprojection

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"media_agent/media_auth/biz/dal"
	domain "media_agent/media_auth/biz/domain/authorizationprojection"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) conn(ctx context.Context) *gorm.DB {
	return dal.FromContext(ctx, r.db).WithContext(ctx)
}

func (r *Repository) ClaimPending(ctx context.Context, limit int) ([]domain.Event, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows []struct {
		ID, AggregateType, AggregateID, EventType string
		Payload                                   []byte
		Sequence                                  int64
		Attempts                                  int
		CreatedAt                                 time.Time
	}
	err := r.conn(ctx).Raw(`
WITH candidates AS (
    SELECT o.id
    FROM media_auth.authorization_outbox o
    WHERE o.delivered_at IS NULL
      AND o.next_attempt_at <= now()
      AND (o.claimed_at IS NULL OR o.claimed_at < now() - interval '30 seconds')
      AND NOT EXISTS (
          SELECT 1 FROM media_auth.authorization_outbox earlier
          WHERE earlier.aggregate_type=o.aggregate_type
            AND earlier.aggregate_id=o.aggregate_id
            AND earlier.delivered_at IS NULL
            AND earlier.sequence < o.sequence
      )
    ORDER BY o.created_at, o.id
    FOR UPDATE SKIP LOCKED
    LIMIT ?
)
UPDATE media_auth.authorization_outbox o
SET claimed_at=now()
FROM candidates c
WHERE o.id=c.id
RETURNING o.id,o.aggregate_type,o.aggregate_id,o.event_type,o.payload,o.sequence,o.attempts,o.created_at`, limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Event, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Event{ID: row.ID, AggregateType: row.AggregateType, AggregateID: row.AggregateID, EventType: row.EventType, Payload: json.RawMessage(row.Payload), Sequence: row.Sequence, Attempts: row.Attempts, CreatedAt: row.CreatedAt})
	}
	return out, nil
}

func (r *Repository) MarkDelivered(ctx context.Context, event domain.Event) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO media_auth.authorization_projection_checkpoints
            (aggregate_type,aggregate_id,sequence,event_id) VALUES (?,?,?,?)
            ON CONFLICT (aggregate_type,aggregate_id) DO UPDATE
            SET sequence=EXCLUDED.sequence,event_id=EXCLUDED.event_id,projected_at=now()
            WHERE media_auth.authorization_projection_checkpoints.sequence < EXCLUDED.sequence`, event.AggregateType, event.AggregateID, event.Sequence, event.ID).Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE media_auth.authorization_outbox SET delivered_at=now(),claimed_at=NULL,last_error=NULL WHERE id=?", event.ID).Error
	})
}

func (r *Repository) MarkFailed(ctx context.Context, event domain.Event, message string, retryAt time.Time) error {
	return r.conn(ctx).Exec("UPDATE media_auth.authorization_outbox SET attempts=attempts+1,last_error=?,next_attempt_at=?,claimed_at=NULL WHERE id=?", message, retryAt, event.ID).Error
}

func (r *Repository) Organization(ctx context.Context, id string) (domain.OrganizationSnapshot, error) {
	var row struct{ ID, Status string }
	if err := r.conn(ctx).Table("media_auth.organizations").Select("id,status").Where("id=?", id).Take(&row).Error; err != nil {
		return domain.OrganizationSnapshot{}, err
	}
	snapshot := domain.OrganizationSnapshot{ID: row.ID, Status: row.Status}
	if err := r.conn(ctx).Table("media_auth.organization_members").Select("user_id,role").Where("organization_id=?", id).Order("user_id").Scan(&snapshot.Members).Error; err != nil {
		return domain.OrganizationSnapshot{}, err
	}
	if err := r.conn(ctx).Table("media_auth.organization_entitlements").Where("organization_id=?", id).Order("feature_key").Pluck("feature_key", &snapshot.Entitlements).Error; err != nil {
		return domain.OrganizationSnapshot{}, err
	}
	return snapshot, nil
}

func (r *Repository) Team(ctx context.Context, id string) (domain.TeamSnapshot, error) {
	var row struct{ ID, OrganizationID, Status, EntitlementMode string }
	err := r.conn(ctx).Table("media_auth.teams").
		Select("id,organization_id,status,entitlement_mode").Where("id = ?", id).Take(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return domain.TeamSnapshot{ID: id, Status: "decommissioned"}, nil
		}
		return domain.TeamSnapshot{}, err
	}
	snapshot := domain.TeamSnapshot{ID: row.ID, OrganizationID: row.OrganizationID, Status: row.Status, EntitlementMode: row.EntitlementMode}
	if err := r.conn(ctx).Table("media_auth.team_members tm").
		Select("tm.user_id,tm.role").
		Joins("JOIN media_auth.organization_members om ON om.organization_id = ? AND om.user_id = tm.user_id", row.OrganizationID).
		Where("tm.team_id = ?", id).Order("tm.user_id").Scan(&snapshot.Members).Error; err != nil {
		return domain.TeamSnapshot{}, err
	}
	if err := r.conn(ctx).Table("media_auth.organization_entitlements").Where("organization_id = ?", row.OrganizationID).
		Order("feature_key").Pluck("feature_key", &snapshot.OrganizationEntitlements).Error; err != nil {
		return domain.TeamSnapshot{}, err
	}
	if err := r.conn(ctx).Table("media_auth.team_entitlements").Where("team_id = ?", id).
		Order("feature_key").Pluck("feature_key", &snapshot.TeamEntitlements).Error; err != nil {
		return domain.TeamSnapshot{}, err
	}
	var organizationStatus string
	if err := r.conn(ctx).Table("media_auth.organizations").Where("id = ?", row.OrganizationID).Pluck("status", &organizationStatus).Error; err != nil {
		return domain.TeamSnapshot{}, err
	}
	if organizationStatus != "active" {
		snapshot.Status = "decommissioned"
	}
	return snapshot, nil
}
