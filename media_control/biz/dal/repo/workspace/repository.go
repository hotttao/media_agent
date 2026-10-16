package workspace

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	domain "media_agent/media_control/biz/domain/workspace"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, w *domain.Workspace, a *domain.AgentConfiguration) error {
	db := dal.FromContext(ctx, r.db)
	if err := db.Create(toWorkspaceRow(w)).Error; err != nil {
		return err
	}
	return db.Create(toAgentRow(a)).Error
}

func (r *Repository) ListOwned(ctx context.Context, owner string) ([]domain.ConfiguredWorkspace, error) {
	var rows []struct {
		model.Workspace
		AgentConfigID                          string `gorm:"column:agent_config_id"`
		AgentID                                string
		ModelConnectionID                      string
		ModelProtocol, ModelName, ModelBaseURL string
		AgentType                              string
		EncryptedAPIKey                        []byte
		ConfigurationVersion                   int64 `gorm:"column:configuration_version"`
	}
	err := dal.FromContext(ctx, r.db).Table("workspaces w").Select("w.*, a.id AS agent_config_id, a.agent_id, a.agent_type, a.model_connection_id, a.model_protocol, a.model_name, a.model_base_url, a.encrypted_api_key, a.version AS configuration_version").Joins("JOIN agent_configurations a ON a.workspace_id = w.id").Where("w.owner_user_id = ? AND w.desired_state <> ?", owner, "deleted").Order("w.updated_at DESC").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]domain.ConfiguredWorkspace, 0, len(rows))
	for _, row := range rows {
		result = append(result, fromJoined(row.Workspace, row.AgentConfigID, row.AgentID, row.AgentType, row.EncryptedAPIKey, row.ConfigurationVersion, row.ModelConnectionID, row.ModelProtocol, row.ModelName, row.ModelBaseURL))
	}
	return result, nil
}

func (r *Repository) ListByTeams(ctx context.Context, teamIDs []string) ([]domain.ConfiguredWorkspace, error) {
	if len(teamIDs) == 0 {
		return []domain.ConfiguredWorkspace{}, nil
	}
	var rows []struct {
		model.Workspace
		AgentConfigID                          string `gorm:"column:agent_config_id"`
		AgentID                                string
		ModelConnectionID                      string
		ModelProtocol, ModelName, ModelBaseURL string
		AgentType                              string
		EncryptedAPIKey                        []byte
		ConfigurationVersion                   int64 `gorm:"column:configuration_version"`
	}
	err := dal.FromContext(ctx, r.db).Table("workspaces w").Select("w.*, a.id AS agent_config_id, a.agent_id, a.agent_type, a.model_connection_id, a.model_protocol, a.model_name, a.model_base_url, a.encrypted_api_key, a.version AS configuration_version").Joins("JOIN agent_configurations a ON a.workspace_id = w.id").Where("w.team_id IN ? AND w.desired_state <> ?", teamIDs, "deleted").Order("w.updated_at DESC").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]domain.ConfiguredWorkspace, 0, len(rows))
	for _, row := range rows {
		result = append(result, fromJoined(row.Workspace, row.AgentConfigID, row.AgentID, row.AgentType, row.EncryptedAPIKey, row.ConfigurationVersion, row.ModelConnectionID, row.ModelProtocol, row.ModelName, row.ModelBaseURL))
	}
	return result, nil
}

func (r *Repository) Get(ctx context.Context, id string) (*domain.ConfiguredWorkspace, error) {
	var w model.Workspace
	db := dal.FromContext(ctx, r.db)
	if err := db.Where("id = ? AND desired_state <> ?", id, "deleted").First(&w).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	var a model.AgentConfiguration
	if err := db.Where("workspace_id = ?", id).First(&a).Error; err != nil {
		return nil, err
	}
	value := fromJoined(w, a.ID, a.AgentID, a.AgentType, a.EncryptedAPIKey, a.Version, a.ModelConnectionID, a.ModelProtocol, a.ModelName, a.ModelBaseURL)
	return &value, nil
}

func (r *Repository) GetOwned(ctx context.Context, id, owner string) (*domain.ConfiguredWorkspace, error) {
	var w model.Workspace
	db := dal.FromContext(ctx, r.db)
	if err := db.Where("id = ? AND owner_user_id = ? AND desired_state <> ?", id, owner, "deleted").First(&w).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	var a model.AgentConfiguration
	if err := db.Where("workspace_id = ?", id).First(&a).Error; err != nil {
		return nil, err
	}
	value := fromJoined(w, a.ID, a.AgentID, a.AgentType, a.EncryptedAPIKey, a.Version, a.ModelConnectionID, a.ModelProtocol, a.ModelName, a.ModelBaseURL)
	return &value, nil
}

func (r *Repository) Update(ctx context.Context, w *domain.Workspace, a *domain.AgentConfiguration) error {
	db := dal.FromContext(ctx, r.db)
	result := db.Model(&model.Workspace{}).Where("id = ? AND desired_state <> ?", w.ID, "deleted").Updates(map[string]any{"name": w.Name, "version": w.Version, "updated_at": w.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	err := db.Model(&model.AgentConfiguration{}).Where("workspace_id = ?", w.ID).Updates(map[string]any{"agent_id": a.AgentID, "agent_type": a.AgentType, "encrypted_api_key": a.EncryptedAPIKey, "model_connection_id": a.ModelConnectionID, "model_protocol": a.ModelProtocol, "model_name": a.ModelName, "model_base_url": a.ModelBaseURL, "version": a.Version, "updated_at": a.UpdatedAt}).Error
	var conflict interface{ SQLState() string }
	if errors.As(err, &conflict) && conflict.SQLState() == "40001" {
		return domain.ErrConflict
	}
	return err
}

func toWorkspaceRow(v *domain.Workspace) *model.Workspace {
	return &model.Workspace{ID: v.ID, OrganizationID: v.OrganizationID, TeamID: v.TeamID, CreatedBy: v.CreatedBy, OwnerUserID: v.OwnerUserID, Name: v.Name, DesiredState: v.DesiredState, Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func toAgentRow(v *domain.AgentConfiguration) *model.AgentConfiguration {
	return &model.AgentConfiguration{ID: v.ID, WorkspaceID: v.WorkspaceID, AgentID: v.AgentID, AgentType: v.AgentType, EncryptedAPIKey: v.EncryptedAPIKey, ModelConnectionID: v.ModelConnectionID, ModelProtocol: v.ModelProtocol, ModelName: v.ModelName, ModelBaseURL: v.ModelBaseURL, Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func fromJoined(w model.Workspace, configID, agentID, agentType string, secret []byte, configVersion int64, connectionID, protocol, modelName, baseURL string) domain.ConfiguredWorkspace {
	return domain.ConfiguredWorkspace{Workspace: domain.Workspace{ID: w.ID, OrganizationID: w.OrganizationID, TeamID: w.TeamID, CreatedBy: w.CreatedBy, OwnerUserID: w.OwnerUserID, Name: w.Name, DesiredState: w.DesiredState, Version: w.Version, CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt}, AgentConfiguration: domain.AgentConfiguration{ID: configID, WorkspaceID: w.ID, AgentID: agentID, AgentType: agentType, EncryptedAPIKey: secret, ModelConnectionID: connectionID, ModelProtocol: protocol, ModelName: modelName, ModelBaseURL: baseURL, Version: configVersion, CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt}}
}

func (r *Repository) GetConfigurationVersion(ctx context.Context, id string, version int64) (*domain.AgentConfiguration, string, error) {
	var row struct {
		AgentConfigID, AgentType, ModelConnectionID string
		ModelProtocol, ModelName, ModelBaseURL      string
		Version                                     int64
		EncryptedAPIKey                             []byte
	}
	db := dal.FromContext(ctx, r.db)
	if err := db.Table("agent_configuration_versions").Where("agent_config_id=? AND version=?", id, version).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", domain.ErrNotFound
		}
		return nil, "", err
	}
	var release struct{ ID string }
	if err := db.Table("agent_metadata_releases").Where("agent_config_id=? AND selection_revision=?", id, version).Take(&release).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", err
	}
	return &domain.AgentConfiguration{ID: id, AgentID: row.AgentType, AgentType: row.AgentType, Version: version, EncryptedAPIKey: row.EncryptedAPIKey, ModelConnectionID: row.ModelConnectionID, ModelProtocol: row.ModelProtocol, ModelName: row.ModelName, ModelBaseURL: row.ModelBaseURL}, release.ID, nil
}

func (r *Repository) ConfirmedContextVersion(ctx context.Context, id string) (int64, error) {
	var row struct{ SelectionRevision int64 }
	err := dal.FromContext(ctx, r.db).Table("agent_metadata_releases").Where("agent_config_id=?", id).Order("selection_revision DESC").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return row.SelectionRevision, err
}
