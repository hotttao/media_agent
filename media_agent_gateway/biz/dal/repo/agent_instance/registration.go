package agent_instance

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"media_agent/media_agent_gateway/biz/dal"
	d "media_agent/media_agent_gateway/biz/domain/agent_instance"
	"time"
)

// RegistrationRepository owns SQL for current and historical instance facts.
type RegistrationRepository struct{ db *gorm.DB }

func NewRegistration(db *gorm.DB) *RegistrationRepository { return &RegistrationRepository{db} }
func (r *RegistrationRepository) Get(ctx context.Context, env, ws, cfg, replica string, lock bool) (*d.Instance, error) {
	db := dal.FromContext(ctx, r.db).Table("agent_instances").Where("environment_id=? AND workspace_id=? AND agent_config_id=? AND replica_id=?", env, ws, cfg, replica)
	if lock {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var result d.Instance
	err := db.Take(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	return &result, err
}
func (r *RegistrationRepository) Historical(ctx context.Context, env, ws, cfg, replica, inc string) (*d.Instance, error) {
	var value d.Instance
	err := dal.FromContext(ctx, r.db).Table("agent_instance_history").Where("environment_id=? AND workspace_id=? AND agent_config_id=? AND replica_id=? AND incarnation_id=?", env, ws, cfg, replica, inc).Take(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	return &value, err
}
func (r *RegistrationRepository) List(ctx context.Context, env, ws, cfg string) ([]d.Instance, error) {
	result := []d.Instance{}
	err := dal.FromContext(ctx, r.db).Table("agent_instances").Where("environment_id=? AND workspace_id=? AND agent_config_id=?", env, ws, cfg).Order("replica_id").Find(&result).Error
	return result, err
}
func (r *RegistrationRepository) Save(ctx context.Context, v *d.Instance, expected int64) error {
	db := dal.FromContext(ctx, r.db)
	if expected == 0 {
		result := db.Table("agent_instances").Clauses(clause.OnConflict{DoNothing: true}).Create(v)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return d.ErrConflict
		}
	} else {
		result := db.Table("agent_instances").Where("environment_id=? AND workspace_id=? AND agent_config_id=? AND replica_id=? AND version=?", v.EnvironmentID, v.WorkspaceID, v.AgentConfigID, v.ReplicaID, expected).Updates(map[string]any{"agent_config_id": v.AgentConfigID, "agent_type": v.AgentType, "incarnation_id": v.IncarnationID, "resource_uid": v.ResourceUID, "resource_generation": v.ResourceGeneration, "resource_name": v.ResourceName, "namespace": v.Namespace, "endpoint": v.Endpoint, "credential_hash": v.CredentialHash, "credential_key_id": v.CredentialKeyID, "admission": v.Admission, "drain_operation_id": v.DrainOperationID, "version": v.Version, "updated_at": v.UpdatedAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return d.ErrConflict
		}
	}
	result := db.Table("agent_instance_history").Clauses(clause.OnConflict{DoNothing: true}).Create(v)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return d.ErrConflict
	}
	return nil
}
func (r *RegistrationRepository) Drain(ctx context.Context, v *d.Instance, operation string) error {
	return dal.FromContext(ctx, r.db).Table("agent_instances").Where("environment_id=? AND workspace_id=? AND agent_config_id=? AND replica_id=? AND version=?", v.EnvironmentID, v.WorkspaceID, v.AgentConfigID, v.ReplicaID, v.Version).Updates(map[string]any{"admission": "draining", "drain_operation_id": operation, "version": v.Version + 1, "updated_at": time.Now().UTC()}).Error
}

func (r *RegistrationRepository) Retire(ctx context.Context, v *d.Instance, operation string) error {
	result := dal.FromContext(ctx, r.db).Table("agent_instances").Where("environment_id=? AND workspace_id=? AND agent_config_id=? AND replica_id=? AND incarnation_id=? AND version=? AND admission='draining' AND drain_operation_id=?", v.EnvironmentID, v.WorkspaceID, v.AgentConfigID, v.ReplicaID, v.IncarnationID, v.Version, operation).Updates(map[string]any{"admission": "retired", "version": v.Version + 1, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return d.ErrConflict
	}
	return nil
}
