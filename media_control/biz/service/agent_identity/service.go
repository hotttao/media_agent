package agent_identity

import (
	"context"
	"errors"
	"strings"

	domain "media_agent/media_control/biz/domain/workspace"
)

var ErrInvalidID = errors.New("agent configuration ID is required")
var ErrIdentityUnavailable = errors.New("agent identity facts unavailable")

type Service struct {
	reader domain.AgentIdentityReader
	teams  domain.TeamAccess
}

func New(reader domain.AgentIdentityReader, teams domain.TeamAccess) *Service {
	return &Service{reader: reader, teams: teams}
}

func (s *Service) Resolve(ctx context.Context, id string) (*domain.AgentIdentity, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrInvalidID
	}
	value, err := s.reader.FindAgentIdentity(ctx, id)
	if err != nil {
		return nil, err
	}
	if value == nil || value.ConfigurationID != id || value.WorkspaceID == "" || value.OwnerUserID == "" || value.OrganizationID == "" || value.TeamID == "" {
		return nil, ErrIdentityUnavailable
	}
	if value.WorkspaceState == "deleted" {
		return nil, domain.ErrNotFound
	}
	if value.WorkspaceState != "running" && value.WorkspaceState != "stopped" {
		return nil, ErrIdentityUnavailable
	}
	decision, err := s.teams.Authorize(ctx, value.OwnerUserID, value.TeamID, "view")
	if err != nil {
		return nil, err
	}
	result := *value
	result.OwnerAuthorized = decision.Allowed && decision.OrganizationID == value.OrganizationID && decision.TeamID == value.TeamID
	return &result, nil
}
