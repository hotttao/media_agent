package template

import (
	"context"
	"errors"
	"sort"

	domain "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/policy/resource"
	"media_agent/media_workflow/biz/shared"
)

func (s *Service) GetWithAccess(ctx context.Context, id, caller string, teams ...string) (*domain.Template, resource.Access, error) {
	var access resource.Access
	if s.authority != nil {
		var err error
		ctx, _, err = shared.PrepareResourceFacts(ctx, caller, s.authority)
		if err != nil {
			return nil, access, err
		}
	}
	t, err := s.Get(ctx, id, caller, teams...)
	if err != nil {
		return nil, access, err
	}
	level, err := s.permission(ctx, t, caller, teams)
	if err != nil {
		return nil, access, err
	}
	_, err = s.requireAuthoring(ctx, t.OrganizationID, caller)
	if err != nil && !errors.Is(err, domain.ErrTemplateForbidden) && !errors.Is(err, shared.ErrResourceAccessDenied) {
		return nil, access, err
	}
	return t, resource.Actions(level, t.Status == domain.StatusArchived, err == nil, t.OrganizationID != "", false), nil
}

func (s *Service) ListGrants(ctx context.Context, id, caller string) ([]*domain.Grant, error) {
	if _, err := s.requireOwner(ctx, id, caller); err != nil {
		return nil, err
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
