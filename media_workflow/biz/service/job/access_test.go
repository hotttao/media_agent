package job

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
	"media_agent/hertz_infra/authn/internaljwt"
	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared/client"
)

type accessProbeEngine struct {
	mockWorkflowEngine
	reads int
}

type accessListRepo struct {
	*mockJobRepo
	query domain.ListQuery
	calls int
}

func (r *accessListRepo) List(_ context.Context, q domain.ListQuery) ([]*domain.Job, int, error) {
	r.query = q
	r.calls++
	return nil, 0, nil
}

func TestAgentJobAccessUsesTrustedWorkspaceAndCurrentPermission(t *testing.T) {
	p := internaljwt.AgentPrincipal{UserID: "owner", ActorID: "Agent:configuration", OrganizationID: "org", WorkspaceID: "workspace", CredentialID: "credential"}
	ctx := internaljwt.ContextWithAgentPrincipal(context.Background(), p)
	svc, base, _ := newTestService(false)
	repo := &accessListRepo{mockJobRepo: base}
	svc.jobRepo = repo
	a := &submitAuthority{allowed: true}
	svc.resumeAccess = a
	_, _, err := svc.List(ctx, domain.ListQuery{SubmittedBy: "forged-owner", OrganizationID: "forged-org", WorkspaceID: "forged-workspace"})
	if err != nil || repo.calls != 1 || repo.query.SubmittedBy != "owner" || repo.query.OrganizationID != "org" || repo.query.WorkspaceID != "workspace" {
		t.Fatal("Agent list used untrusted scope", err)
	}
	a.allowed = false
	if _, _, err := svc.List(ctx, domain.ListQuery{}); !errors.Is(err, ErrSubmitForbidden) || repo.calls != 1 {
		t.Fatal("revoked execution permission reached Job repository")
	}
	base.jobs["job"] = &domain.Job{ID: "job", SubmittedBy: "owner", OrganizationID: "org", WorkspaceID: "workspace", Status: domain.JobSucceeded}
	if _, _, err := svc.GetForUser(ctx, "job", "owner", "org"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("permission removal did not revoke Job access")
	}
	a.allowed = true
	if _, _, err := svc.GetForUser(ctx, "job", "owner", "org"); err != nil {
		t.Fatal("authorized Agent cannot read its Workspace Job", err)
	}
	base.jobs["job"].WorkspaceID = "other-workspace"
	if _, _, err := svc.GetForUser(ctx, "job", "owner", "org"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("same-org foreign Workspace exposed")
	}
}

type submitAuthority struct {
	workspace            *client.WorkspaceContext
	lookupErr, accessErr error
	allowed              bool
}

func TestUserSubmitRequiresAuthorizedWorkspaceAndDerivesOrganization(t *testing.T) {
	for _, tc := range []struct {
		name, organization, workspace string
		allowed                       bool
		want                          error
	}{
		{"derive enterprise", "", "workspace", true, nil},
		{"explicit enterprise", "org", "workspace", true, nil},
		{"missing workspace", "org", "", true, ErrSubmitForbidden},
		{"foreign enterprise", "foreign", "workspace", true, ErrSubmitForbidden},
		{"foreign workspace", "org", "other", true, ErrSubmitForbidden},
		{"permission removed", "org", "workspace", false, ErrSubmitForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _, _, _, _ := submissionFixture()
			a := &submitAuthority{workspace: &client.WorkspaceContext{ID: "workspace", OrganizationID: "org"}, allowed: tc.allowed}
			svc.historyWorkspaces, svc.resumeAccess = a, a
			job, err := svc.Submit(context.Background(), SubmitCommand{
				WorkflowID: "wf1", SubmittedBy: "owner", OrganizationID: tc.organization, WorkspaceID: tc.workspace,
				Input: domain.JobInput{Materials: []domain.MaterialData{{Name: "scene", URL: "https://example.test/media/materials/org/scene.png"}}},
			})
			if tc.want != nil {
				if !errors.Is(err, tc.want) || job != nil || len(repo.jobs) != 0 {
					t.Fatal("unauthorized ordinary submission created a Job", err)
				}
			} else if err != nil || job == nil || job.OrganizationID != "org" || job.WorkspaceID != "workspace" {
				t.Fatal("ordinary submission lost authoritative tenant attribution", err)
			}
		})
	}
}

func (a *submitAuthority) GetWorkspaceForUser(context.Context, string, string) (*client.WorkspaceContext, error) {
	return a.workspace, a.lookupErr
}
func (a *submitAuthority) CanUseWorkflow(context.Context, string, string) (bool, error) {
	return a.allowed, a.accessErr
}

func TestDelegatedSubmitRequiresCurrentWorkspacePermission(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Service, *submitAuthority, *SubmitCommand)
		want   error
	}{
		{"missing organization", func(_ *Service, _ *submitAuthority, c *SubmitCommand) { c.OrganizationID = "" }, ErrSubmitForbidden},
		{"missing authority", func(s *Service, _ *submitAuthority, _ *SubmitCommand) { s.historyWorkspaces = nil }, ErrSubmitAuthorityUnavailable},
		{"lookup unavailable", func(_ *Service, a *submitAuthority, _ *SubmitCommand) { a.lookupErr = errors.New("unavailable") }, ErrSubmitAuthorityUnavailable},
		{"different organization", func(_ *Service, a *submitAuthority, _ *SubmitCommand) { a.workspace.OrganizationID = "other" }, ErrSubmitForbidden},
		{"different workspace", func(_ *Service, a *submitAuthority, _ *SubmitCommand) { a.workspace.ID = "other" }, ErrSubmitForbidden},
		{"permission removed", func(_ *Service, a *submitAuthority, _ *SubmitCommand) { a.allowed = false }, ErrSubmitForbidden},
		{"permission unavailable", func(_ *Service, a *submitAuthority, _ *SubmitCommand) { a.accessErr = errors.New("unavailable") }, ErrSubmitAuthorityUnavailable},
		{"private workflow", func(s *Service, _ *submitAuthority, _ *SubmitCommand) {
			s.wfRepo.(*mockWorkflowLookup).wf.CreatedBy = "other"
		}, ErrSubmitForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := newTestService(false)
			svc.wfRepo.(*mockWorkflowLookup).wf.CreatedBy = "owner"
			a := &submitAuthority{workspace: &client.WorkspaceContext{ID: "workspace", OrganizationID: "org"}, allowed: true}
			svc.historyWorkspaces = a
			svc.resumeAccess = a
			cmd := SubmitCommand{WorkflowID: "wf1", SubmittedBy: "owner", OrganizationID: "org", WorkspaceID: "workspace", Delegated: true}
			tc.change(svc, a, &cmd)
			_, err := svc.Submit(context.Background(), cmd)
			if !errors.Is(err, tc.want) || len(repo.jobs) != 0 {
				t.Fatal("denied request persisted a Job or returned the wrong error", err)
			}
		})
	}
	for _, public := range []bool{false, true} {
		svc, _, _, _, _, _ := submissionFixture()
		wf := svc.wfRepo.(*mockWorkflowLookup).wf
		wf.CreatedBy = "owner"
		if public {
			wf.CreatedBy = "other"
			wf.Visibility = wfdomain.VisibilityPublic
		}
		a := &submitAuthority{workspace: &client.WorkspaceContext{ID: "workspace", OrganizationID: "org"}, allowed: true}
		svc.historyWorkspaces = a
		svc.resumeAccess = a
		job, err := svc.Submit(context.Background(), SubmitCommand{WorkflowID: "wf1", SubmittedBy: "owner", OrganizationID: "org", WorkspaceID: "workspace", Delegated: true, Input: domain.JobInput{Materials: []domain.MaterialData{{Name: "scene", URL: "https://example.test/media/materials/org/scene.png"}}}})
		if err != nil || job == nil || job.OrganizationID != "org" || job.WorkspaceID != "workspace" {
			t.Fatal("authorized execution lost its trusted tenant attribution", err)
		}
	}
}

func (e *accessProbeEngine) Get(context.Context, string) (*domain.EngineExecution, error) {
	e.reads++
	return nil, errors.New("engine must not be consulted before authorization")
}

func TestGetForUserAuthorizesBeforeConductorReconciliation(t *testing.T) {
	for _, tc := range []struct{ name, user, organization, storedOwner, storedOrganization string }{
		{"missing caller", "", "org", "owner", "org"},
		{"different owner", "owner", "org", "other", "org"},
		{"different organization", "owner", "org", "owner", "other"},
		{"historical job without organization", "owner", "org", "owner", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := newTestService(false)
			engine := &accessProbeEngine{}
			svc.engine = engine
			repo.jobs["job"] = &domain.Job{ID: "job", SubmittedBy: tc.storedOwner, OrganizationID: tc.storedOrganization, Status: domain.JobRunning, ConductorWorkflowID: "execution"}
			job, steps, err := svc.GetForUser(context.Background(), "job", tc.user, tc.organization)
			if !errors.Is(err, gorm.ErrRecordNotFound) || job != nil || steps != nil || engine.reads != 0 {
				t.Fatal("unauthorized read exposed a Job or consulted Conductor", err)
			}
		})
	}
	for _, org := range []string{"org", ""} {
		svc, repo, _ := newTestService(false)
		repo.jobs["job"] = &domain.Job{ID: "job", SubmittedBy: "owner", OrganizationID: "org", Status: domain.JobSucceeded}
		job, _, err := svc.GetForUser(context.Background(), "job", "owner", org)
		if err != nil || job == nil || job.ID != "job" {
			t.Fatal("authorized owner cannot read the Job", err)
		}
	}
}
