package platform_administration

import (
	"context"
	"errors"
	"strings"
	"time"
)

type AdminWorkspace struct {
	WorkspaceID, OwnerUserID, Name, DesiredState string
	WorkspaceVersion                             int64
	AgentID, AgentType                           string
	ContextVersion                               int64
	EnvironmentID, PlacementStatus               string
	UpdatedAt                                    time.Time
}

type AdminWorkspaceFilter struct {
	OwnerUserID, DesiredState, AgentType string
	PageSize, PageOffset                 int32
}

type AdminWorkspaceRepository interface {
	ListAdminWorkspaces(context.Context, AdminWorkspaceFilter) ([]AdminWorkspace, int64, error)
}

func ValidateAdminWorkspaceFilter(owner, desiredState, agentType string, pageSize, pageOffset int32) (AdminWorkspaceFilter, error) {
	owner, desiredState, agentType = strings.TrimSpace(owner), strings.TrimSpace(desiredState), strings.TrimSpace(agentType)
	if desiredState != "" && desiredState != "running" && desiredState != "stopped" && desiredState != "deleted" {
		return AdminWorkspaceFilter{}, errors.New("desired_state must be running, stopped or deleted")
	}
	if agentType != "" && agentType != "codex" && agentType != "claude-code" {
		return AdminWorkspaceFilter{}, errors.New("agent_type must be codex or claude-code")
	}
	if pageSize == 0 {
		pageSize = 50
	}
	if pageSize < 1 || pageSize > 200 || pageOffset < 0 {
		return AdminWorkspaceFilter{}, errors.New("page_size must be between 1 and 200 and page_offset must be non-negative")
	}
	return AdminWorkspaceFilter{OwnerUserID: owner, DesiredState: desiredState, AgentType: agentType, PageSize: pageSize, PageOffset: pageOffset}, nil
}
