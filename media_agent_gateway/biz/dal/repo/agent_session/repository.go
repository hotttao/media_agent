package agent_session

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"media_agent/media_agent_gateway/biz/dal"
	"media_agent/media_agent_gateway/biz/dal/model"
	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) FindLatestUsable(ctx context.Context, userID string) (*domain.Session, error) {
	var row model.AgentSession
	err := dal.FromContext(ctx, r.db).Where("user_id = ? AND status IN ?", userID, []string{domain.StatusStarting, domain.StatusActive}).Order("updated_at DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toSession(row), nil
}

func (r *Repository) TransitionStatus(ctx context.Context, sessionID, from, to string, updatedAt time.Time) error {
	result := dal.FromContext(ctx, r.db).Model(&model.AgentSession{}).
		Where("id = ? AND status = ?", sessionID, from).
		Updates(map[string]any{"status": to, "updated_at": updatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repository) CreateSession(ctx context.Context, session *domain.Session) error {
	return dal.FromContext(ctx, r.db).Create(&model.AgentSession{
		ID: session.ID, UserID: session.UserID, WorkspaceID: session.WorkspaceID,
		AgentID: session.AgentID, AgentType: session.AgentType, Title: session.Title,
		TokenHash: session.TokenHash, TokenKeyID: session.TokenKeyID, BindingPhase: session.BindingPhase, ExecutionEpoch: session.ExecutionEpoch, ExpectedExecutionEpoch: session.ExpectedExecutionEpoch,
		ContextVersion: session.ContextVersion, Status: session.Status, AgentConfigID: session.AgentConfigID, MetadataTargetContextVersion: session.MetadataTargetContextVersion,
		CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt,
	}).Error
}

func (r *Repository) ListOwned(ctx context.Context, userID, workspaceID string) ([]domain.Session, error) {
	var rows []model.AgentSession
	db := dal.FromContext(ctx, r.db).Where("user_id = ? AND status <> ?", userID, "deleted")
	if workspaceID != "" {
		db = db.Where("workspace_id = ?", workspaceID)
	}
	if err := db.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Session, 0, len(rows))
	for _, row := range rows {
		result = append(result, *toSession(row))
	}
	return result, nil
}

func (r *Repository) GetOwned(ctx context.Context, sessionID, userID string) (*domain.Session, error) {
	var row model.AgentSession
	err := dal.FromContext(ctx, r.db).Where("id = ?", sessionID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.UserID != userID {
		return nil, domain.ErrForbidden
	}
	return toSession(row), nil
}

func toSession(row model.AgentSession) *domain.Session {
	return &domain.Session{
		ID: row.ID, UserID: row.UserID, WorkspaceID: row.WorkspaceID,
		AgentID: row.AgentID, AgentType: row.AgentType, Title: row.Title,
		EnvironmentID: row.EnvironmentID, ReplicaID: row.ReplicaID, IncarnationID: row.IncarnationID, BindingPhase: row.BindingPhase, TransferID: row.TransferID, TargetReplicaID: row.TargetReplicaID, TargetIncarnationID: row.TargetIncarnationID, BindingVersion: row.BindingVersion, ExecutionEpoch: row.ExecutionEpoch, ExpectedExecutionEpoch: row.ExpectedExecutionEpoch, TokenHash: row.TokenHash, TokenKeyID: row.TokenKeyID,
		ContextVersion: row.ContextVersion, AgentConfigID: row.AgentConfigID, MetadataTargetContextVersion: row.MetadataTargetContextVersion, Status: row.Status,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (r *Repository) GetSession(ctx context.Context, id string) (*domain.Session, error) {
	var row model.AgentSession
	err := dal.FromContext(ctx, r.db).Where("id=?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toSession(row), nil
}
func (r *Repository) cas(ctx context.Context, id string, version int64, values map[string]any) (*domain.Session, error) {
	values["binding_version"] = version + 1
	values["updated_at"] = time.Now().UTC()
	result := dal.FromContext(ctx, r.db).Model(&model.AgentSession{}).Where("id=? AND binding_version=?", id, version).Updates(values)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, domain.ErrBindingConflict
	}
	return r.GetSession(ctx, id)
}
func (r *Repository) AssignBinding(ctx context.Context, id string, version int64, b domain.Binding) (*domain.Session, error) {
	return r.cas(ctx, id, version, map[string]any{"environment_id": b.EnvironmentID, "replica_id": b.ReplicaID, "incarnation_id": b.IncarnationID, "execution_epoch": b.ExecutionEpoch, "binding_phase": "active"})
}
func (r *Repository) BeginTransfer(ctx context.Context, id string, version int64, b domain.Binding, transfer string) (*domain.Session, error) {
	return r.cas(ctx, id, version, map[string]any{"binding_phase": "transferring", "transfer_id": transfer, "target_replica_id": b.ReplicaID, "target_incarnation_id": b.IncarnationID, "expected_execution_epoch": b.ExecutionEpoch})
}
func (r *Repository) CompleteTransfer(ctx context.Context, id string, version int64, transfer string, actual domain.Binding) (*domain.Session, error) {
	current, err := r.GetSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if current.BindingPhase != "transferring" || current.TransferID != transfer || actual.ExecutionEpoch != current.ExpectedExecutionEpoch+1 || actual.EnvironmentID != current.EnvironmentID {
		return nil, domain.ErrBindingConflict
	}
	return r.cas(ctx, id, version, map[string]any{"binding_phase": "active", "replica_id": actual.ReplicaID, "incarnation_id": actual.IncarnationID, "execution_epoch": actual.ExecutionEpoch, "target_replica_id": "", "target_incarnation_id": ""})
}
func (r *Repository) EnsureCredential(ctx context.Context, id, hash, key string) (*domain.Session, error) {
	result := dal.FromContext(ctx, r.db).Model(&model.AgentSession{}).Where("id=? AND token_hash=''", id).Updates(map[string]any{"token_hash": hash, "token_key_id": key})
	if result.Error != nil {
		return nil, result.Error
	}
	return r.GetSession(ctx, id)
}
func (r *Repository) PendingTransfers(ctx context.Context, limit int) ([]domain.Session, error) {
	var rows []model.AgentSession
	if err := dal.FromContext(ctx, r.db).Where("binding_phase='transferring'").Order("updated_at").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Session, 0, len(rows))
	for _, row := range rows {
		result = append(result, *toSession(row))
	}
	return result, nil
}

func (r *Repository) EnsureIdentity(ctx context.Context, id, configuration string) (*domain.Session, error) {
	result := dal.FromContext(ctx, r.db).Model(&model.AgentSession{}).Where("id=? AND agent_config_id=''", id).Update("agent_config_id", configuration)
	if result.Error != nil {
		return nil, result.Error
	}
	session, err := r.GetSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if session.AgentConfigID != configuration {
		return nil, domain.ErrBindingConflict
	}
	return session, nil
}
func (r *Repository) BoundSessions(ctx context.Context, after string, limit int) ([]domain.Session, error) {
	var rows []model.AgentSession
	query := dal.FromContext(ctx, r.db).Where("binding_phase='active' AND status IN ?", []string{domain.StatusStarting, domain.StatusActive})
	if after != "" {
		query = query.Where("id>?", after)
	}
	err := query.Order("id").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]domain.Session, 0, len(rows))
	for _, row := range rows {
		result = append(result, *toSession(row))
	}
	return result, nil
}

func (r *Repository) RetargetTransfer(ctx context.Context, id string, version int64, transfer string, target domain.Binding) (*domain.Session, error) {
	current, err := r.GetSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if current.BindingPhase != "transferring" || current.TransferID != transfer || current.EnvironmentID != target.EnvironmentID {
		return nil, domain.ErrBindingConflict
	}
	return r.cas(ctx, id, version, map[string]any{"target_replica_id": target.ReplicaID, "target_incarnation_id": target.IncarnationID})
}

func (r *Repository) ByTokenHash(ctx context.Context, hash string) (*domain.Session, error) {
	var row model.AgentSession
	e := dal.FromContext(ctx, r.db).Where("token_hash=? AND status=?", hash, domain.StatusActive).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, domain.ErrForbidden
	}
	if e != nil {
		return nil, e
	}
	return toSession(row), nil
}
