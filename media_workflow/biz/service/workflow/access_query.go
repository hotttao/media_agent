package workflow

import (
	"context"
	"errors"
	"sort"

	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/policy/resource"
	"media_agent/media_workflow/biz/shared"
)

func (s *Service) GetWithAccess(ctx context.Context, id, caller string, teams ...string) (*domain.Workflow, resource.Access, error) {
	var access resource.Access
	if s.authority != nil {
		var err error
		ctx, _, err = shared.PrepareResourceFacts(ctx, caller, s.authority)
		if err != nil {
			return nil, access, err
		}
	}
	w, err := s.Get(ctx, id, caller, teams...)
	if err != nil {
		return nil, access, err
	}
	level, err := s.permission(ctx, w, caller, teams)
	if err != nil {
		return nil, access, err
	}
	err = s.requireAuthoring(ctx, w, caller)
	if err != nil && !errors.Is(err, domain.ErrWorkflowForbidden) && !errors.Is(err, shared.ErrResourceAccessDenied) {
		return nil, access, err
	}
	// A current registration receipt is required; draft and publishing are not runnable.
	runnable := w.Status == domain.StatusPublished && w.PublicationStatus == "registered"
	// Versions registered before publication receipts existed remain usable.
	// The registration must be an actual retained fact, never inferred from status.
	if w.Status == domain.StatusPublished && w.PublicationStatus == "" && s.versions != nil {
		frozen, readErr := s.versions.GetDefinitionVersion(ctx, w.ID, w.Version)
		if readErr != nil {
			return nil, access, shared.ErrResourceAuthorityUnavailable
		}
		runnable = frozen != nil && frozen.Registered
	}
	return w, resource.Actions(level, w.Status == domain.StatusArchived, err == nil, w.OrganizationID != "", runnable), nil
}

func (s *Service) ListGrants(ctx context.Context, id, caller string) ([]*domain.Grant, error) {
	w, err := s.Get(ctx, id, caller)
	if err != nil {
		return nil, err
	}
	if caller == "" || w.CreatedBy != caller {
		return nil, domain.ErrWorkflowForbidden
	}
	if s.grants == nil {
		return nil, shared.ErrResourceAuthorityUnavailable
	}
	rows, err := s.grants.ListGrants(ctx, id)
	if err != nil {
		return nil, shared.ErrResourceAuthorityUnavailable
	}
	rows = append([]*domain.Grant{}, rows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].TeamID < rows[j].TeamID })
	return rows, nil
}
