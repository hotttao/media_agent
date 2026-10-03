package workspace

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	domain "media_agent/media_control/biz/domain/workspace"
)

// FindAgentIdentity loads only identity facts in one statement. It must never
// fetch/decrypt encrypted_api_key or expose runtime configuration.
func (r *Repository) FindAgentIdentity(ctx context.Context, id string) (*domain.AgentIdentity, error) {
	var row struct {
		ConfigurationID, WorkspaceID, OwnerUserID, OrganizationID, TeamID, WorkspaceState string
	}
	err := dal.FromContext(ctx, r.db).Table("agent_configurations a").
		Select("a.id AS configuration_id, w.id AS workspace_id, w.owner_user_id, w.organization_id, w.team_id, w.desired_state AS workspace_state").
		Joins("JOIN workspaces w ON w.id = a.workspace_id").
		Where("a.id = ? AND w.desired_state <> ?", id, "deleted").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &domain.AgentIdentity{ConfigurationID: row.ConfigurationID, WorkspaceID: row.WorkspaceID, OwnerUserID: row.OwnerUserID,
		OrganizationID: row.OrganizationID, TeamID: row.TeamID, WorkspaceState: row.WorkspaceState}, nil
}
