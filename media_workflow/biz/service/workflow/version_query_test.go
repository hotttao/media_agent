package workflow

import (
	"context"
	"errors"
	"reflect"
	"testing"

	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared"
)

type versionQueryRepo struct{ reads int }

func (r *versionQueryRepo) GetDefinitionVersion(_ context.Context, id string, version int) (*domain.DefinitionVersion, error) {
	r.reads++
	if version != 1 {
		return nil, domain.ErrVersionNotFound
	}
	return &domain.DefinitionVersion{Source: "snapshot", Registered: true, Definition: &domain.Workflow{ID: id, Version: version, Name: "old name", Visibility: domain.VisibilityPublic, InputSchema: []domain.InputSchemaEntry{{Name: "old_input", ResourceType: "TEXT", Required: true}}, Steps: []domain.Step{{Key: "old_step", TemplateID: "tpl", TemplateVersion: 1}}}}, nil
}

func TestWorkflowVersionUsesCurrentBoundaryAndFrozenDefinition(t *testing.T) {
	head := &departmentResourceRepo{resource: &domain.Workflow{ID: "wf", Version: 2, Name: "new name", OrganizationID: "org", WorkspaceID: "workspace", CreatedBy: "owner", Visibility: domain.VisibilityTeam}}
	versions := &versionQueryRepo{}
	authority := &resourceAuthorityStub{facts: shared.ResourceFacts{OrganizationIDs: []string{"org"}, WorkspaceIDs: []string{"workspace"}, TeamIDs: []string{"dept"}}}
	svc := NewService(head, departmentGrantRepo{permission: domain.PermissionEditor}, nil, nil, nil)
	svc.ConfigureResourceAuthority(authority)
	svc.ConfigureVersions(versions)
	snapshot, access, err := svc.GetVersionForCaller(context.Background(), "wf", 1, "member")
	if err != nil || snapshot.Definition.Name != "old name" || snapshot.Definition.InputSchema[0].Name != "old_input" || snapshot.Definition.Visibility != domain.VisibilityTeam || snapshot.Definition.Status != domain.StatusPublished || access.MyPermission != "editor" || !reflect.DeepEqual(access.AllowedActions, []string{"read"}) {
		t.Fatal("historical definition/access drift", snapshot, access, err)
	}
	authority.facts.TeamIDs = nil
	if _, _, err := svc.GetVersionForCaller(context.Background(), "wf", 1, "member", "dept"); !errors.Is(err, domain.ErrWorkflowForbidden) || versions.reads != 1 {
		t.Fatal("revoked member read history", err)
	}
	authority.facts.OrganizationIDs = nil
	if _, _, err := svc.GetVersionForCaller(context.Background(), "wf", 1, "owner"); !errors.Is(err, domain.ErrWorkflowForbidden) || versions.reads != 1 {
		t.Fatal("owner crossed enterprise", err)
	}
	authority.err = shared.ErrResourceAuthorityUnavailable
	if _, _, err := svc.GetVersionForCaller(context.Background(), "wf", 1, "owner"); !errors.Is(err, shared.ErrResourceAuthorityUnavailable) || versions.reads != 1 {
		t.Fatal("outage read history", err)
	}
	authority.err = nil
	authority.facts.OrganizationIDs = []string{"org"}
	if _, _, err := svc.GetVersionForCaller(context.Background(), "wf", 2, "owner"); !errors.Is(err, domain.ErrVersionNotFound) {
		t.Fatal("missing version invented", err)
	}
	if _, _, err := svc.GetVersionForCaller(context.Background(), "wf", 3, "owner"); !errors.Is(err, domain.ErrVersionNotFound) || versions.reads != 2 {
		t.Fatal("future version read", err)
	}
}
