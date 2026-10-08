package workflow

import (
	"context"
	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared"
)

func (s *Service) requireAuthoring(ctx context.Context, w *domain.Workflow, caller string) error {
	if s.authority == nil {
		if w.OrganizationID != "" || w.WorkspaceID != "" {
			return shared.ErrResourceAuthorityUnavailable
		}
		return nil
	}
	if w.OrganizationID == "" && w.WorkspaceID == "" {
		_, facts, err := shared.PrepareResourceFacts(ctx, caller, s.authority)
		if err != nil {
			return err
		}
		if len(facts.WorkflowOrganizationIDs) == 0 {
			return domain.ErrWorkflowForbidden
		}
		return nil
	}
	if w.OrganizationID == "" || w.WorkspaceID == "" {
		return domain.ErrWorkflowForbidden
	}
	workspace, err := s.authority.GetWorkspaceForUser(ctx, w.WorkspaceID, caller)
	if err != nil {
		return err
	}
	if workspace == nil || workspace.ID != w.WorkspaceID || workspace.OrganizationID != w.OrganizationID {
		return domain.ErrWorkflowForbidden
	}
	return nil
}
