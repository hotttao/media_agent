package runtime_environment

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"media_agent/media_control/biz/dal"
	d "media_agent/media_control/biz/domain/runtime_environment"
)

func (r *Repository) Workload(ctx context.Context, ws string, lock bool) (*d.WorkloadTarget, error) {
	db := dal.FromContext(ctx, r.db).Table("agent_workload_targets").Where("workspace_id=?", ws)
	if lock {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row d.WorkloadTarget
	e := db.Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	return &row, e
}
func (r *Repository) WorkloadRequest(ctx context.Context, ws, key string) (*d.WorkloadRequest, error) {
	var row struct {
		RequestHash string
		Target      json.RawMessage
	}
	e := dal.FromContext(ctx, r.db).Table("agent_workload_requests").Select("request_hash,target").Where("workspace_id=? AND idempotency_key=?", ws, key).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	result := &d.WorkloadRequest{WorkspaceID: ws, IdempotencyKey: key, RequestHash: row.RequestHash}
	e = json.Unmarshal(row.Target, &result.Target)
	return result, e
}
func (r *Repository) SaveWorkload(ctx context.Context, v d.WorkloadTarget, expected int64, request d.WorkloadRequest) error {
	db := dal.FromContext(ctx, r.db)
	values := map[string]any{"workspace_id": v.WorkspaceID, "agent_config_id": v.AgentConfigID, "environment_id": v.EnvironmentID, "owner_user_id": v.OwnerUserID, "spec": string(v.Spec), "replicas": v.Replicas, "desired_replicas": v.DesiredReplicas, "workspace_version": v.WorkspaceVersion, "version": v.Version}
	var result *gorm.DB
	if expected == 0 {
		result = db.Table("agent_workload_targets").Clauses(clause.OnConflict{DoNothing: true}).Create(values)
	} else {
		result = db.Table("agent_workload_targets").Where("workspace_id=? AND version=?", v.WorkspaceID, expected).Updates(values)
	}
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return d.ErrConflict
	}
	encoded, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return db.Table("agent_workload_requests").Create(map[string]any{"workspace_id": v.WorkspaceID, "idempotency_key": request.IdempotencyKey, "request_hash": request.RequestHash, "target": string(encoded)}).Error
}
func (r *Repository) ListWorkloads(ctx context.Context, after string, limit int) ([]d.WorkloadTarget, error) {
	var result []d.WorkloadTarget
	e := dal.FromContext(ctx, r.db).Table("agent_workload_targets").Where("workspace_id>?", after).Order("workspace_id").Limit(limit).Find(&result).Error
	return result, e
}
