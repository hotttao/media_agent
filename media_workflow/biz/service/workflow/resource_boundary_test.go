package workflow

import (
	"context"
	"errors"
	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared"
	"testing"
)

type resourceAuthorityStub struct {
	shared.ResourceAuthority
	facts           shared.ResourceFacts
	err             error
	workspace       *shared.ResourceWorkspace
	workspaceErr    error
	workspaceChecks int
}

func (a *resourceAuthorityStub) Facts(context.Context, string) (shared.ResourceFacts, error) {
	return a.facts, a.err
}

func (a *resourceAuthorityStub) GetWorkspaceForUser(_ context.Context, id, user string) (*shared.ResourceWorkspace, error) {
	a.workspaceChecks++
	return a.workspace, a.workspaceErr
}

func TestWorkflowEnterpriseWorkspaceAndCurrentGrantBoundary(t *testing.T) {
	repo := &departmentResourceRepo{resource: &domain.Workflow{ID: "wf", OrganizationID: "org", WorkspaceID: "workspace", CreatedBy: "owner", Visibility: domain.VisibilityTeam}}
	a := &resourceAuthorityStub{facts: shared.ResourceFacts{OrganizationIDs: []string{"org"}, TeamIDs: []string{"dept"}, WorkspaceIDs: []string{"workspace"}}}
	svc := NewService(repo, departmentGrantRepo{permission: domain.PermissionViewer}, nil, nil, nil)
	svc.ConfigureResourceAuthority(a)
	if err := svc.AuthorizeExecution(context.Background(), "wf", "org", "workspace", "member"); err != nil {
		t.Fatal(err)
	}
	if err := svc.AuthorizeExecution(context.Background(), "wf", "org", "other", "member"); !errors.Is(err, domain.ErrWorkflowForbidden) {
		t.Fatal("wrong execution workspace admitted")
	}
	a.facts.TeamIDs = nil
	if _, err := svc.Get(context.Background(), "wf", "member", "dept"); !errors.Is(err, domain.ErrWorkflowForbidden) {
		t.Fatal("stale department admitted")
	}
	repo.resource.Visibility = domain.VisibilityPublic
	a.facts.OrganizationIDs = []string{"foreign"}
	if _, err := svc.Get(context.Background(), "wf", "member"); !errors.Is(err, domain.ErrWorkflowForbidden) {
		t.Fatal("public crossed enterprise")
	}
	a.facts.OrganizationIDs = []string{"org"}
	a.facts.WorkspaceIDs = nil
	if _, err := svc.Get(context.Background(), "wf", "owner"); !errors.Is(err, domain.ErrWorkflowForbidden) {
		t.Fatal("owner bypassed workspace boundary")
	}
	a.err = shared.ErrResourceAuthorityUnavailable
	if _, _, err := svc.List(context.Background(), domain.ListQuery{}, "owner"); !errors.Is(err, shared.ErrResourceAuthorityUnavailable) {
		t.Fatal("authority failure listed resources")
	}
}

func TestWorkflowAuthoringChecksExactWorkspaceFunctionality(t *testing.T) {
	for _, caller := range []string{"owner", "editor"} {
		repo := &departmentResourceRepo{resource: &domain.Workflow{ID: "wf", OrganizationID: "org", WorkspaceID: "workspace", CreatedBy: "owner", Visibility: domain.VisibilityTeam, Status: domain.StatusDraft}}
		a := &resourceAuthorityStub{facts: shared.ResourceFacts{OrganizationIDs: []string{"org"}, WorkspaceIDs: []string{"workspace"}, TeamIDs: []string{"dept"}, WorkflowOrganizationIDs: []string{"org"}}, workspaceErr: shared.ErrResourceAccessDenied}
		svc := NewService(repo, departmentGrantRepo{permission: domain.PermissionEditor}, nil, nil, nil)
		svc.ConfigureResourceAuthority(a)
		if _, err := svc.Update(context.Background(), UpdateCommand{ID: "wf", CallerID: caller}); !errors.Is(err, shared.ErrResourceAccessDenied) || repo.writes != 0 {
			t.Fatal("another department's functionality authorized editing", err)
		}
		if caller == "owner" {
			if _, err := svc.Publish(context.Background(), "wf", caller); !errors.Is(err, shared.ErrResourceAccessDenied) {
				t.Fatal("owner published in disabled workspace", err)
			}
		}
		if _, err := svc.Get(context.Background(), "wf", caller); err != nil {
			t.Fatal("disabled execution removed independent read", err)
		}
		a.workspaceErr = nil
		a.workspace = &shared.ResourceWorkspace{ID: "other", OrganizationID: "org"}
		if err := svc.requireAuthoring(context.Background(), repo.resource, caller); !errors.Is(err, domain.ErrWorkflowForbidden) {
			t.Fatal("wrong Workspace authority response accepted")
		}
		a.workspace = &shared.ResourceWorkspace{ID: "workspace", OrganizationID: "org"}
		if err := svc.requireAuthoring(context.Background(), repo.resource, caller); err != nil {
			t.Fatal(err)
		}
		a.workspaceErr = shared.ErrResourceAuthorityUnavailable
		if err := svc.requireAuthoring(context.Background(), repo.resource, caller); !errors.Is(err, shared.ErrResourceAuthorityUnavailable) {
			t.Fatal("authority outage authorized editing")
		}
	}
}

func TestReadableWorkflowNonOwnerManagementReturnsForbidden(t *testing.T) {
	for _, permission := range []domain.Permission{domain.PermissionViewer, domain.PermissionEditor} {
		repo := &departmentResourceRepo{resource: &domain.Workflow{ID: "wf", OrganizationID: "org", WorkspaceID: "workspace", CreatedBy: "owner", Visibility: domain.VisibilityTeam}}
		svc := NewService(repo, departmentGrantRepo{permission: permission}, nil, nil, nil)
		svc.ConfigureResourceAuthority(&resourceAuthorityStub{facts: shared.ResourceFacts{OrganizationIDs: []string{"org"}, TeamIDs: []string{"dept"}, WorkspaceIDs: []string{"workspace"}}})
		if _, err := svc.ChangeVisibility(context.Background(), "wf", "public", "member"); !errors.Is(err, domain.ErrWorkflowForbidden) {
			t.Fatal("readable non-owner visibility must be forbidden", err)
		}
		if _, err := svc.Revoke(context.Background(), "wf", "dept", "member"); !errors.Is(err, domain.ErrWorkflowForbidden) {
			t.Fatal("readable non-owner revoke must be forbidden", err)
		}
	}
}
