//go:build e2e

package e2e

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/datatypes"
	"media_agent/media_workflow/biz/dal/model"
	tplrepo "media_agent/media_workflow/biz/dal/repo/template"
	wfrepo "media_agent/media_workflow/biz/dal/repo/workflow"
	tpldomain "media_agent/media_workflow/biz/domain/template"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	tplsvc "media_agent/media_workflow/biz/service/template"
	wfsvc "media_agent/media_workflow/biz/service/workflow"
)

// Real isolated storage, with explicit membership facts. No production users
// or grants are changed; this does not test IAM organization membership.
func TestResourceDepartmentPermissionsAndAuthorizedPagination(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	timestamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, fixture := range []struct {
		id, owner, visibility string
		deleted               bool
	}{
		{"1_owned", "member", "private", false},
		{"2_public", "owner", "public", false},
		{"3_shared", "owner", "team", false},
		{"4_wrong_department", "owner", "team", false},
		{"5_private_with_grant", "owner", "private", false},
		{"6_deleted", "owner", "public", true},
	} {
		tpl := &model.Template{ID: fixture.id, Scope: "acl_test", Name: fixture.id, PromptBody: "test snapshot", OutputAssetKind: "first_frame", Status: "draft", Visibility: fixture.visibility, Version: 1, CreatedBy: fixture.owner, CreatedAt: timestamp, UpdatedAt: timestamp, InvocationMapping: datatypes.JSON(`{}`)}
		wf := &model.Workflow{ID: fixture.id, Scope: "acl_test", Name: fixture.id, OutputStepKey: "output", Status: "draft", Visibility: fixture.visibility, Version: 1, CreatedBy: fixture.owner, CreatedAt: timestamp, UpdatedAt: timestamp, InputSchema: datatypes.JSON(`[]`)}
		if fixture.deleted {
			tpl.DeletedAt = &timestamp
			wf.DeletedAt = &timestamp
		}
		if db.Create(tpl).Error != nil || db.Create(wf).Error != nil {
			t.Fatal("resource fixture creation failed")
		}
	}
	templates, templateGrants := tplrepo.NewRepo(db), tplrepo.NewGrantRepo(db)
	workflows, workflowGrants := wfrepo.NewRepo(db), wfrepo.NewGrantRepo(db)
	for _, id := range []string{"3_shared", "4_wrong_department", "5_private_with_grant"} {
		team := "dept"
		if id == "4_wrong_department" {
			team = "other"
		}
		if templateGrants.AddGrant(ctx, &tpldomain.Grant{TemplateID: id, TeamID: team, Permission: tpldomain.PermissionViewer, GrantedBy: "owner"}) != nil || workflowGrants.AddGrant(ctx, &wfdomain.Grant{WorkflowID: id, TeamID: team, Permission: wfdomain.PermissionViewer, GrantedBy: "owner"}) != nil {
			t.Fatal("grant fixture creation failed")
		}
	}
	ts := tplsvc.NewService(templates, templateGrants, nil, nil, nil)
	ws := wfsvc.NewService(workflows, workflowGrants, nil, nil, nil)
	for _, offset := range []int{0, 2} {
		tplItems, tplTotal, err := ts.List(ctx, tpldomain.ListQuery{Scope: "acl_test", Limit: 2, Offset: offset}, "member", "dept")
		if err != nil {
			t.Fatal(err)
		}
		wfItems, wfTotal, err := ws.List(ctx, wfdomain.ListQuery{Scope: "acl_test", Limit: 2, Offset: offset}, "member", "dept")
		if err != nil {
			t.Fatal(err)
		}
		wantLen := 2
		if offset == 2 {
			wantLen = 1
		}
		if tplTotal != 3 || wfTotal != 3 || len(tplItems) != wantLen || len(wfItems) != wantLen {
			t.Fatal("authorization was applied after count/pagination")
		}
		for i, item := range tplItems {
			if item.ID != wfItems[i].ID || item.ID == "4_wrong_department" || item.ID == "5_private_with_grant" || item.ID == "6_deleted" {
				t.Fatal("inaccessible resource leaked into page")
			}
		}
	}
	if _, err := ts.Get(ctx, "3_shared", "member", "dept"); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Get(ctx, "3_shared", "member", "dept"); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.GetVersionForCaller(ctx, "3_shared", 1, "member", "dept"); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Archive(ctx, "3_shared", "member", "dept"); !errors.Is(err, tpldomain.ErrTemplateForbidden) {
		t.Fatal("viewer archived template")
	}
	if _, err := ws.Archive(ctx, "3_shared", "member", "dept"); !errors.Is(err, wfdomain.ErrWorkflowForbidden) {
		t.Fatal("viewer archived workflow")
	}
	if templateGrants.AddGrant(ctx, &tpldomain.Grant{TemplateID: "3_shared", TeamID: "dept", Permission: tpldomain.PermissionEditor, GrantedBy: "owner"}) != nil || workflowGrants.AddGrant(ctx, &wfdomain.Grant{WorkflowID: "3_shared", TeamID: "dept", Permission: wfdomain.PermissionEditor, GrantedBy: "owner"}) != nil {
		t.Fatal("editor grant upgrade failed")
	}
	if _, err := ts.Archive(ctx, "3_shared", "member", "dept"); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Archive(ctx, "3_shared", "member", "dept"); err != nil {
		t.Fatal(err)
	}
	if templateGrants.RemoveGrant(ctx, "3_shared", "dept") != nil || workflowGrants.RemoveGrant(ctx, "3_shared", "dept") != nil {
		t.Fatal("grant revoke failed")
	}
	if _, err := ts.Get(ctx, "3_shared", "member", "dept"); !errors.Is(err, tpldomain.ErrTemplateForbidden) {
		t.Fatal("revoked template grant still readable")
	}
	if _, err := ws.Get(ctx, "3_shared", "member", "dept"); !errors.Is(err, wfdomain.ErrWorkflowForbidden) {
		t.Fatal("revoked workflow grant still readable")
	}
	_, tplTotal, err := ts.List(ctx, tpldomain.ListQuery{Scope: "acl_test"}, "member", "dept")
	if err != nil || tplTotal != 2 {
		t.Fatal("template grant revoke not reflected in total")
	}
	_, wfTotal, err := ws.List(ctx, wfdomain.ListQuery{Scope: "acl_test"}, "member", "dept")
	if err != nil || wfTotal != 2 {
		t.Fatal("workflow grant revoke not reflected in total")
	}
}
