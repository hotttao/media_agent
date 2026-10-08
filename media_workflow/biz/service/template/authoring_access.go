package template

import (
	"context"
	domain "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/shared"
)

func (s *Service) requireAuthoring(ctx context.Context, organization, caller string) (context.Context, error) {
	if s.authority == nil {
		if organization != "" {
			return ctx, shared.ErrResourceAuthorityUnavailable
		}
		return ctx, nil
	}
	ctx, facts, err := shared.PrepareResourceFacts(ctx, caller, s.authority)
	if err != nil {
		return ctx, err
	}
	if organization == "" {
		if len(facts.WorkflowOrganizationIDs) == 0 {
			return ctx, domain.ErrTemplateForbidden
		}
	} else if !shared.HasResourceID(facts.WorkflowOrganizationIDs, organization) {
		return ctx, domain.ErrTemplateForbidden
	}
	return ctx, nil
}
