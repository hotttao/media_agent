package workflow

import (
	"context"
	"errors"
	"slices"
	"testing"

	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared"
)

func TestWorkflowAccessProjectionAndGrants(t *testing.T) {
	repo := &departmentResourceRepo{resource: &domain.Workflow{ID: "wf", OrganizationID: "org", WorkspaceID: "workspace", CreatedBy: "owner", Visibility: domain.VisibilityPublic, Status: domain.StatusPublished, PublicationStatus: "registered"}}
	authority := &resourceAuthorityStub{facts: shared.ResourceFacts{OrganizationIDs: []string{"org"}, WorkspaceIDs: []string{"workspace"}}, workspace: &shared.ResourceWorkspace{ID: "workspace", OrganizationID: "org"}}
	svc := NewService(repo, departmentGrantRepo{permission: domain.PermissionEditor}, nil, nil, nil)
	svc.ConfigureResourceAuthority(authority)
	for _, tc := range []struct {
		caller, permission string
		teams              []string
	}{{"owner", "owner", nil}, {"editor", "editor", []string{"dept"}}, {"viewer", "viewer", nil}} {
		authority.facts.TeamIDs = tc.teams
		_, access, err := svc.GetWithAccess(context.Background(), "wf", tc.caller)
		if err != nil || access.MyPermission != tc.permission || !slices.Contains(access.AllowedActions, "run") {
			t.Fatal("valid registration not runnable", access, err)
		}
		if slices.Contains(access.AllowedActions, "publish") != (tc.caller == "owner") {
			t.Fatal("non-owner published")
		}
		rows, err := svc.ListGrants(context.Background(), "wf", tc.caller)
		if tc.caller == "owner" {
			if err != nil || len(rows) != 1 {
				t.Fatal("owner grants denied", err)
			}
		} else if !errors.Is(err, domain.ErrWorkflowForbidden) {
			t.Fatal("non-owner grant disclosure", err)
		}
	}
	repo.resource.PublicationStatus = "pending"
	_, access, err := svc.GetWithAccess(context.Background(), "wf", "owner")
	if err != nil || slices.Contains(access.AllowedActions, "run") {
		t.Fatal("pending registration runnable")
	}
	repo.resource.Version = 1
	repo.resource.PublicationStatus = ""
	svc.ConfigureVersions(&versionQueryRepo{})
	_, access, err = svc.GetWithAccess(context.Background(), "wf", "owner")
	if err != nil || !slices.Contains(access.AllowedActions, "run") {
		t.Fatal("retained old registration denied", err)
	}
	authority.workspaceErr = shared.ErrResourceAccessDenied
	_, access, err = svc.GetWithAccess(context.Background(), "wf", "owner")
	if err != nil || slices.Contains(access.AllowedActions, "update") || !slices.Contains(access.AllowedActions, "archive") {
		t.Fatal("revoked functionality projection", access, err)
	}
	authority.workspaceErr = shared.ErrResourceAuthorityUnavailable
	if _, _, err := svc.GetWithAccess(context.Background(), "wf", "owner"); !errors.Is(err, shared.ErrResourceAuthorityUnavailable) {
		t.Fatal("outage returned projection", err)
	}
	authority.err = shared.ErrResourceAuthorityUnavailable
	if _, err := svc.ListGrants(context.Background(), "wf", "owner"); !errors.Is(err, shared.ErrResourceAuthorityUnavailable) {
		t.Fatal("outage disclosed grants", err)
	}
}
