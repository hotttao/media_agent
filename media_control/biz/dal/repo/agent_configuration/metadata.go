package agent_configuration

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	logger "gorm.io/gorm/logger"
	"media_agent/media_control/biz/dal"
	d "media_agent/media_control/biz/domain/agent_configuration"
	"time"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository {
	return &Repository{db.Session(&gorm.Session{Logger: db.Logger.LogMode(logger.Silent)})}
}
func (r *Repository) Resource(ctx context.Context, ref d.ResourceReference) (*d.ResourceVersion, error) {
	var row struct {
		Kind, ResourceID, OwnerUserID string
		WorkspaceID                   *string
		Version                       int64
		Payload                       json.RawMessage
	}
	err := r.fromContext(ctx).Table("catalog_resource_versions").Where("kind=? AND resource_id=? AND version=?", ref.Kind, ref.ID, ref.Version).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v := &d.ResourceVersion{ResourceReference: ref, OwnerUserID: row.OwnerUserID, Payload: row.Payload}
	if row.WorkspaceID != nil {
		v.WorkspaceID = *row.WorkspaceID
	}
	return v, nil
}

type releaseRow struct {
	ID, WorkspaceID, AgentConfigID, RequestHash, IdempotencyKey, CreatorID string
	SelectionRevision                                                      int64
	Manifest                                                               json.RawMessage
	CreatedAt                                                              time.Time
}

func decode(row releaseRow) (*d.Release, error) {
	var snapshot d.Snapshot
	if err := json.Unmarshal(row.Manifest, &snapshot); err != nil {
		return nil, err
	}
	return &d.Release{Snapshot: snapshot, RequestHash: row.RequestHash, IdempotencyKey: row.IdempotencyKey, CreatorID: row.CreatorID, Manifest: row.Manifest, CreatedAt: row.CreatedAt}, nil
}
func (r *Repository) read(ctx context.Context, condition string, args ...any) (*d.Release, error) {
	var row releaseRow
	err := r.fromContext(ctx).Table("agent_metadata_releases").Where(condition, args...).Order("selection_revision DESC").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return decode(row)
}
func (r *Repository) FindByKey(ctx context.Context, id, key string) (*d.Release, error) {
	return r.read(ctx, "agent_config_id=? AND idempotency_key=?", id, key)
}
func (r *Repository) Current(ctx context.Context, ws, id string) (*d.Release, error) {
	return r.read(ctx, "workspace_id=? AND agent_config_id=?", ws, id)
}
func (r *Repository) Find(ctx context.Context, ws, id, target string) (*d.Release, error) {
	return r.read(ctx, "workspace_id=? AND agent_config_id=? AND id=?", ws, id, target)
}
func (r *Repository) SaveSelection(ctx context.Context, v *d.Release, expected int64, notice d.Notice) error {
	db := r.fromContext(ctx)
	var row struct{ Version int64 }
	if err := db.Table("agent_configurations").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND workspace_id=?", v.AgentConfigID, v.WorkspaceID).Take(&row).Error; err != nil {
		return err
	}
	if previous, err := r.FindByKey(ctx, v.AgentConfigID, v.IdempotencyKey); err == nil {
		if previous.RequestHash != v.RequestHash {
			return d.ErrConflict
		}
		return nil
	} else if !errors.Is(err, d.ErrNotFound) {
		return err
	}
	if row.Version != expected {
		return d.ErrConflict
	}
	result := db.Table("agent_configurations").Where("id=? AND version=?", v.AgentConfigID, expected).Updates(map[string]any{"version": v.SelectionRevision, "updated_at": v.CreatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return d.ErrConflict
	}
	if err := db.Table("agent_metadata_releases").Create(map[string]any{"id": v.TargetContextVersion, "workspace_id": v.WorkspaceID, "agent_config_id": v.AgentConfigID, "selection_revision": v.SelectionRevision, "request_hash": v.RequestHash, "idempotency_key": v.IdempotencyKey, "creator_id": v.CreatorID, "manifest": string(v.Manifest), "created_at": v.CreatedAt}).Error; err != nil {
		return err
	}
	key := hex.EncodeToString([]byte(v.WorkspaceID)) + "." + hex.EncodeToString([]byte(v.AgentConfigID))
	return db.Table("metadata_notification_outbox").Create(map[string]any{"event_id": notice.EventID, "release_id": v.TargetContextVersion, "routing_key": key, "payload": string(notice.Payload)}).Error
}
func (r *Repository) SaveReport(ctx context.Context, v d.ApplicationResult, hash string) error {
	db := r.fromContext(ctx)
	result := db.Table("metadata_application_reports").Clauses(clause.OnConflict{DoNothing: true}).Create(map[string]any{"report_id": v.ReportID, "workspace_id": v.WorkspaceID, "agent_config_id": v.AgentConfigID, "selection_revision": v.SelectionRevision, "target_context_version": v.TargetContextVersion, "replica_id": v.ReplicaID, "incarnation_id": v.IncarnationID, "status": v.Status, "applied_context_version": v.AppliedContextVersion, "error_code": v.ErrorCode, "content_hash": hash})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var row struct{ ContentHash string }
	if err := db.Table("metadata_application_reports").Where("report_id=?", v.ReportID).Take(&row).Error; err != nil {
		return err
	}
	if row.ContentHash != hash {
		return d.ErrConflict
	}
	return nil
}
func (r *Repository) Reports(ctx context.Context, id string, revision int64) ([]d.ApplicationResult, error) {
	var rows []struct {
		ReportID, WorkspaceID, AgentConfigID, TargetContextVersion, ReplicaID, IncarnationID, Status, AppliedContextVersion, ErrorCode string
		SelectionRevision                                                                                                              int64
	}
	err := r.fromContext(ctx).Table("metadata_application_reports").Where("agent_config_id=? AND selection_revision=?", id, revision).Order("created_at,report_id").Find(&rows).Error
	values := make([]d.ApplicationResult, 0, len(rows))
	for _, v := range rows {
		values = append(values, d.ApplicationResult(v))
	}
	return values, err
}
func (r *Repository) ClaimNotices(ctx context.Context, limit int) ([]d.Notice, error) {
	values := []d.Notice{}
	err := func() error {
		db := r.fromContext(ctx)
		var rows []struct {
			EventID, RoutingKey string
			Payload             json.RawMessage
			Attempts            int
		}
		if err := db.Table("metadata_notification_outbox").Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("delivered_at IS NULL AND available_at<=now() AND (lease_until IS NULL OR lease_until<now())").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		for _, v := range rows {
			token := uuid.NewString()
			if err := db.Table("metadata_notification_outbox").Where("event_id=?", v.EventID).Updates(map[string]any{"lease_token": token, "lease_until": time.Now().Add(15 * time.Second)}).Error; err != nil {
				return err
			}
			values = append(values, d.Notice{EventID: v.EventID, RoutingKey: v.RoutingKey, LeaseToken: token, Payload: v.Payload, Attempts: v.Attempts})
		}
		return nil
	}()
	return values, err
}
func (r *Repository) FinishNotice(ctx context.Context, v d.Notice, ok bool) error {
	updates := map[string]any{"lease_until": nil, "lease_token": nil}
	if ok {
		updates["delivered_at"] = time.Now()
	} else {
		attempt := v.Attempts + 1
		delay := time.Second * time.Duration(1<<min(attempt, 5))
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		updates["attempts"] = attempt
		updates["available_at"] = time.Now().Add(delay)
	}
	return r.db.WithContext(ctx).Table("metadata_notification_outbox").Where("event_id=? AND lease_token=?", v.EventID, v.LeaseToken).Updates(updates).Error
}

func (r *Repository) fromContext(ctx context.Context) *gorm.DB {
	return dal.FromContext(ctx, r.db).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
}
