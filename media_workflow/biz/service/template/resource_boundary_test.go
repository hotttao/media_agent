package template

import (
	"context"
	"errors"
	domain "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/shared"
	"testing"
)

type authorityStub struct {
	shared.ResourceAuthority
	facts   shared.ResourceFacts
	err     error
	teamOrg string
}

func (a *authorityStub) Facts(context.Context, string) (shared.ResourceFacts, error) {
	return a.facts, a.err
}
func (a *authorityStub) TeamOrganization(context.Context, string, string) (string, error) {
	return a.teamOrg, a.err
}

func TestTemplateCurrentOrganizationAndMembershipBoundary(t *testing.T) {
	repo := &accessRepo{value: &domain.Template{ID: "tpl", OrganizationID: "org", CreatedBy: "owner", Visibility: domain.VisibilityPublic}}
	a := &authorityStub{facts: shared.ResourceFacts{OrganizationIDs: []string{"foreign"}, TeamIDs: []string{"dept"}}}
	svc := NewService(repo, departmentGrants{rows: []*domain.Grant{{TeamID: "dept", Permission: domain.PermissionEditor}}}, nil, nil, nil)
	svc.ConfigureResourceAuthority(a)
	if _, err := svc.Get(context.Background(), "tpl", "owner"); !errors.Is(err, domain.ErrTemplateForbidden) {
		t.Fatal("owner bypassed enterprise boundary")
	}
	a.facts.OrganizationIDs = []string{"org"}
	if _, err := svc.Get(context.Background(), "tpl", "viewer"); err != nil {
		t.Fatal("same enterprise public viewer denied", err)
	}
	repo.value.Visibility = domain.VisibilityTeam
	if _, err := svc.Get(context.Background(), "tpl", "viewer", "forged"); err != nil {
		t.Fatal("current department grant ignored", err)
	}
	a.facts.TeamIDs = nil
	if _, err := svc.Get(context.Background(), "tpl", "viewer", "dept"); !errors.Is(err, domain.ErrTemplateForbidden) {
		t.Fatal("caller-supplied stale membership authorized")
	}
	a.err = shared.ErrResourceAuthorityUnavailable
	if _, err := svc.GetVersionForCaller(context.Background(), "tpl", 1, "owner"); !errors.Is(err, shared.ErrResourceAuthorityUnavailable) || repo.versionReads != 0 {
		t.Fatal("authority failure read private snapshot", err)
	}
}

func TestUnboundLegacyTemplateIsNeverPublicOrShareable(t *testing.T) {
	repo := &accessRepo{value: &domain.Template{ID: "old", CreatedBy: "owner", Visibility: domain.VisibilityPublic}}
	a := &authorityStub{facts: shared.ResourceFacts{OrganizationIDs: []string{"org"}}, teamOrg: "org"}
	svc := NewService(repo, nil, nil, nil, nil)
	svc.ConfigureResourceAuthority(a)
	if _, err := svc.Get(context.Background(), "old", "owner"); err != nil {
		t.Fatal("legacy owner lost read", err)
	}
	if _, err := svc.Get(context.Background(), "old", "viewer"); !errors.Is(err, domain.ErrTemplateForbidden) {
		t.Fatal("legacy public escaped unknown enterprise")
	}
	if _, err := svc.ChangeVisibility(context.Background(), "old", "team", "owner"); !errors.Is(err, domain.ErrTemplateForbidden) {
		t.Fatal("unbound resource shared", err)
	}
	if _, err := svc.Grant(context.Background(), "old", "dept", "viewer", "owner"); !errors.Is(err, domain.ErrTemplateForbidden) {
		t.Fatal("unbound resource granted", err)
	}
}

func TestTemplateFeatureRevocationBlocksOwnerAndEditorAuthoring(t *testing.T) {
	for _, caller := range []string{"owner", "editor"} {
		repo := &accessRepo{value: &domain.Template{ID: "tpl", OrganizationID: "org", CreatedBy: "owner", Visibility: domain.VisibilityTeam, Status: domain.StatusDraft}}
		a := &authorityStub{facts: shared.ResourceFacts{OrganizationIDs: []string{"org"}, TeamIDs: []string{"dept"}, WorkflowOrganizationIDs: []string{"other"}}}
		svc := NewService(repo, departmentGrants{rows: []*domain.Grant{{TeamID: "dept", Permission: domain.PermissionEditor}}}, nil, nil, nil)
		svc.ConfigureResourceAuthority(a)
		if _, err := svc.Update(context.Background(), UpdateCommand{ID: "tpl", CallerID: caller}); !errors.Is(err, domain.ErrTemplateForbidden) || repo.writes != 0 {
			t.Fatal("resource role bypassed revoked functionality", err)
		}
		if caller == "owner" {
			if _, err := svc.Publish(context.Background(), "tpl", caller); !errors.Is(err, domain.ErrTemplateForbidden) || repo.writes != 0 {
				t.Fatal("owner published without functional entitlement", err)
			}
		}
		if _, err := svc.Get(context.Background(), "tpl", caller); err != nil {
			t.Fatal("functional revoke incorrectly removed independent read", err)
		}
		a.facts.WorkflowOrganizationIDs = []string{"org"}
		if _, err := svc.requireAuthoring(context.Background(), "org", caller); err != nil {
			t.Fatal("restored functionality was not observed", err)
		}
		a.err = shared.ErrResourceAuthorityUnavailable
		if _, err := svc.requireAuthoring(context.Background(), "org", caller); !errors.Is(err, shared.ErrResourceAuthorityUnavailable) {
			t.Fatal("authority outage confused with permission", err)
		}
	}
}
