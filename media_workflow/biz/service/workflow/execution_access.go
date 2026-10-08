package workflow

import (
	"context"
	domain "media_agent/media_workflow/biz/domain/workflow"
)

// AuthorizeExecution checks the definition, never its source templates. An
// already published compilation is the sole executable snapshot.
func (s *Service) AuthorizeExecution(ctx context.Context, id, org, workspace, user string) error {
	w, err := s.Get(ctx, id, user)
	if err != nil {
		return err
	}
	if w.OrganizationID != "" && (w.OrganizationID != org || w.WorkspaceID != workspace) {
		return domain.ErrWorkflowForbidden
	}
	return nil
}
