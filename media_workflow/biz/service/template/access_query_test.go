package template

import (
	"context"
	"errors"
	"slices"
	"testing"

	domain "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/shared"
)

func TestTemplateAccessProjectionAndGrants(t *testing.T) {
	repo := &accessRepo{value: &domain.Template{ID: "tpl", OrganizationID: "org", CreatedBy: "owner", Visibility: domain.VisibilityPublic, Status: domain.StatusDraft}}
	grants := departmentGrants{rows: []*domain.Grant{{TeamID: "z", Permission: domain.PermissionViewer}, {TeamID: "dept", Permission: domain.PermissionEditor}}}
	authority := &authorityStub{facts: shared.ResourceFacts{OrganizationIDs: []string{"org"}, TeamIDs: []string{"dept"}, WorkflowOrganizationIDs: []string{"org"}}}
	svc := NewService(repo, grants, nil, nil, nil)
	svc.ConfigureResourceAuthority(authority)
	for _, tc := range []struct {
		caller, permission string
		teams              []string
	}{{"owner", "owner", nil}, {"editor", "editor", []string{"dept"}}, {"viewer", "viewer", nil}} {
		t.Run(tc.permission, func(t *testing.T) {
			authority.facts.TeamIDs = tc.teams
			_, access, err := svc.GetWithAccess(context.Background(), "tpl", tc.caller, "forged-team")
			if err != nil || access.MyPermission != tc.permission {
				t.Fatalf("projection %+v %v", access, err)
			}
			if slices.Contains(access.AllowedActions, "publish") != (tc.caller == "owner") {
				t.Fatal("publish role drift")
			}
			rows, err := svc.ListGrants(context.Background(), "tpl", tc.caller)
			if tc.caller == "owner" {
				if err != nil || len(rows) != 2 || rows[0].TeamID != "dept" || grants.rows[0].TeamID != "z" {
					t.Fatal("owner list not sorted or mutated storage", err)
				}
			} else if !errors.Is(err, domain.ErrTemplateForbidden) {
				t.Fatal("non-owner read grant list", err)
			}
		})
	}
	authority.facts.WorkflowOrganizationIDs = nil
	_, access, err := svc.GetWithAccess(context.Background(), "tpl", "owner")
	if err != nil || slices.Contains(access.AllowedActions, "update") || !slices.Contains(access.AllowedActions, "archive") {
		t.Fatal("feature revoke removed cleanup or authorized edit", access, err)
	}
	authority.err = shared.ErrResourceAuthorityUnavailable
	if _, _, err := svc.GetWithAccess(context.Background(), "tpl", "owner"); !errors.Is(err, shared.ErrResourceAuthorityUnavailable) {
		t.Fatal("outage became permission projection", err)
	}
	if _, err := svc.ListGrants(context.Background(), "tpl", "owner"); !errors.Is(err, shared.ErrResourceAuthorityUnavailable) {
		t.Fatal("outage disclosed grants", err)
	}
}
