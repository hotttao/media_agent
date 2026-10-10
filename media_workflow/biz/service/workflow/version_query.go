package workflow

import (
	"context"

	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/policy/resource"
	"media_agent/media_workflow/biz/shared"
)

func (s *Service) GetVersionForCaller(ctx context.Context, id string, version int, caller string, teams ...string) (*domain.DefinitionVersion, resource.Access, error) {
	var access resource.Access
	if s.authority != nil {
		var err error
		ctx, _, err = shared.PrepareResourceFacts(ctx, caller, s.authority)
		if err != nil {
			return nil, access, err
		}
	}
	// Always current boundary and grants, never the historical access state.
	head, err := s.Get(ctx, id, caller, teams...)
	if err != nil {
		return nil, access, err
	}
	if version <= 0 || version > head.Version {
		return nil, access, domain.ErrVersionNotFound
	}
	if s.versions == nil {
		return nil, access, shared.ErrResourceAuthorityUnavailable
	}
	snapshot, err := s.versions.GetDefinitionVersion(ctx, id, version)
	if err != nil {
		return nil, access, err
	}
	if snapshot == nil || snapshot.Definition == nil || snapshot.Definition.ID != id || snapshot.Definition.Version != version {
		return nil, access, shared.ErrResourceAuthorityUnavailable
	}
	level, err := s.permission(ctx, head, caller, teams)
	if err != nil {
		return nil, access, err
	}
	w := snapshot.Definition
	w.OrganizationID, w.WorkspaceID, w.Visibility, w.CreatedBy, w.CreatedAt = head.OrganizationID, head.WorkspaceID, head.Visibility, head.CreatedBy, head.CreatedAt
	w.Status = domain.StatusDraft
	if snapshot.Registered {
		w.Status = domain.StatusPublished
	}
	if version == head.Version {
		w.Status, w.PublicationID, w.PublicationStatus = head.Status, head.PublicationID, head.PublicationStatus
	} else {
		if _, err = s.publicationView(ctx, w); err != nil {
			return nil, access, err
		}
		if !snapshot.Registered {
			switch w.PublicationStatus {
			case "pending":
				w.Status = domain.StatusPublishing
			case "failed":
				w.Status = domain.StatusPublishFailed
			}
		}
	}
	access = resource.Access{MyPermission: resource.Actions(level, false, false, false, false).MyPermission, AllowedActions: []string{"read"}}
	return snapshot, access, nil
}
