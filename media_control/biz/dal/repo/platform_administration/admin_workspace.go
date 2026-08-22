package platform_administration

import (
	"context"
	"time"

	"media_agent/media_control/biz/dal"
	domain "media_agent/media_control/biz/domain/platform_administration"
)

func (r *Repository) ListAdminWorkspaces(ctx context.Context, filter domain.AdminWorkspaceFilter) ([]domain.AdminWorkspace, int64, error) {
	db := dal.FromContext(ctx, r.db)
	query := db.Table("workspaces AS w").Joins("JOIN agent_configurations AS ac ON ac.workspace_id = w.id").Joins("LEFT JOIN workspace_placements AS wp ON wp.workspace_id = w.id")
	if filter.OwnerUserID != "" {
		query = query.Where("w.owner_user_id = ?", filter.OwnerUserID)
	}
	if filter.DesiredState != "" {
		query = query.Where("w.desired_state = ?", filter.DesiredState)
	}
	if filter.AgentType != "" {
		query = query.Where("ac.agent_type = ?", filter.AgentType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []struct {
		WorkspaceID, OwnerUserID, Name, DesiredState string
		WorkspaceVersion                             int64
		AgentID, AgentType                           string
		ContextVersion                               int64
		EnvironmentID, PlacementStatus               string
		UpdatedAt                                    time.Time
	}
	err := query.Select(`w.id AS workspace_id, w.owner_user_id, w.name, w.desired_state, w.version AS workspace_version,
            ac.agent_id, ac.agent_type, ac.version AS context_version,
            COALESCE(wp.environment_id, '') AS environment_id, COALESCE(wp.status, 'unplaced') AS placement_status,
            GREATEST(w.updated_at, ac.updated_at, COALESCE(wp.updated_at, w.updated_at)) AS updated_at`).
		Order("updated_at DESC, workspace_id ASC").Limit(int(filter.PageSize)).Offset(int(filter.PageOffset)).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	result := make([]domain.AdminWorkspace, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.AdminWorkspace{WorkspaceID: row.WorkspaceID, OwnerUserID: row.OwnerUserID, Name: row.Name, DesiredState: row.DesiredState, WorkspaceVersion: row.WorkspaceVersion, AgentID: row.AgentID, AgentType: row.AgentType, ContextVersion: row.ContextVersion, EnvironmentID: row.EnvironmentID, PlacementStatus: row.PlacementStatus, UpdatedAt: row.UpdatedAt})
	}
	return result, total, nil
}
