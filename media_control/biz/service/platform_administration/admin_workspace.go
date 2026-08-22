package platform_administration

import (
	"context"

	domain "media_agent/media_control/biz/domain/platform_administration"
)

type AdminWorkspaceService struct {
	repo domain.AdminWorkspaceRepository
}

func NewAdminWorkspaceService(repo domain.AdminWorkspaceRepository) *AdminWorkspaceService {
	return &AdminWorkspaceService{repo: repo}
}
func (s *AdminWorkspaceService) List(ctx context.Context, actor domain.Actor, owner, desiredState, agentType string, pageSize, pageOffset int32) ([]domain.AdminWorkspace, int64, domain.AdminWorkspaceFilter, error) {
	if err := actor.ValidateAdmin(); err != nil {
		return nil, 0, domain.AdminWorkspaceFilter{}, err
	}
	filter, err := domain.ValidateAdminWorkspaceFilter(owner, desiredState, agentType, pageSize, pageOffset)
	if err != nil {
		return nil, 0, domain.AdminWorkspaceFilter{}, err
	}
	values, total, err := s.repo.ListAdminWorkspaces(ctx, filter)
	return values, total, filter, err
}
