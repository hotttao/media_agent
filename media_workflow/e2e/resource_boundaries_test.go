//go:build e2e

package e2e

import (
	"context"
	"errors"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	tplrepo "media_agent/media_workflow/biz/dal/repo/template"
	wfrepo "media_agent/media_workflow/biz/dal/repo/workflow"
	tpldomain "media_agent/media_workflow/biz/domain/template"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	tplsvc "media_agent/media_workflow/biz/service/template"
	wfsvc "media_agent/media_workflow/biz/service/workflow"
	"media_agent/media_workflow/biz/shared"
	"testing"
	"time"
)

type boundaryAuthority struct {
	shared.ResourceAuthority
	facts     shared.ResourceFacts
	targetOrg string
}

type delayedTemplateLock struct {
	tpldomain.Repo
	db      *gorm.DB
	reached chan struct{}
	release chan struct{}
}

func (r *delayedTemplateLock) GetForUpdate(ctx context.Context, id string) (*tpldomain.Template, error) {
	close(r.reached)
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return tplrepo.NewRepo(r.db).GetForUpdate(ctx, id)
}

func TestResourceRevokeWinsBeforeEditorWriteLock(t *testing.T) {
	db := newDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.Create(&model.Template{ID: "race_tpl", OrganizationID: "org", Scope: "acl", Name: "race", PromptBody: "original", OutputAssetKind: "first_frame", Status: "published", Visibility: "team", Version: 1, CreatedBy: "owner", InvocationMapping: datatypes.JSON(`{}`)}).Error; err != nil {
		t.Fatal(err)
	}
	grants := tplrepo.NewGrantRepo(db)
	if err := grants.AddGrant(ctx, &tpldomain.Grant{TemplateID: "race_tpl", TeamID: "dept", Permission: tpldomain.PermissionEditor, GrantedBy: "owner"}); err != nil {
		t.Fatal(err)
	}
	authority := &boundaryAuthority{facts: shared.ResourceFacts{OrganizationIDs: []string{"org"}, TeamIDs: []string{"dept"}, WorkflowOrganizationIDs: []string{"org"}}, targetOrg: "org"}
	delayed := &delayedTemplateLock{Repo: tplrepo.NewRepo(db), db: db, reached: make(chan struct{}), release: make(chan struct{})}
	editor := tplsvc.NewService(delayed, grants, dal.NewTxRunner(db), nil, nil)
	editor.ConfigureResourceAuthority(authority)
	outcome := make(chan error, 1)
	go func() { _, err := editor.Archive(ctx, "race_tpl", "editor", "dept"); outcome <- err }()
	select {
	case <-delayed.reached:
	case <-ctx.Done():
		t.Fatal("editor did not reach lock")
	}
	owner := tplsvc.NewService(tplrepo.NewRepo(db), grants, dal.NewTxRunner(db), nil, nil)
	owner.ConfigureResourceAuthority(authority)
	if _, err := owner.Revoke(ctx, "race_tpl", "dept", "owner"); err != nil {
		close(delayed.release)
		t.Fatal(err)
	}
	close(delayed.release)
	select {
	case err := <-outcome:
		if !errors.Is(err, tpldomain.ErrTemplateForbidden) {
			t.Fatal("stale precheck allowed revoked editor write", err)
		}
	case <-ctx.Done():
		t.Fatal("editor did not finish")
	}
	current, err := tplrepo.NewRepo(db).Get(ctx, "race_tpl")
	if err != nil || current.Status != tpldomain.StatusPublished {
		t.Fatal("revoked editor changed resource", err)
	}
}

func (a *boundaryAuthority) Facts(context.Context, string) (shared.ResourceFacts, error) {
	return a.facts, nil
}
func (a *boundaryAuthority) TeamOrganization(context.Context, string, string) (string, error) {
	return a.targetOrg, nil
}

// Real isolated persistence, explicit IAM facts. No deployed service or user
// data is mutated; current IAM HTTP behavior is covered separately.
func TestBoundResourcePaginationPublicationAndRevokePersistence(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	tx := dal.NewTxRunner(db)
	now := time.Now()
	for _, f := range []struct{ id, org, ws, vis, owner string }{{"owned", "org", "ws", "private", "member"}, {"public", "org", "ws", "public", "owner"}, {"shared", "org", "ws", "team", "owner"}, {"foreign", "other", "other_ws", "public", "owner"}, {"unknown", "", "", "public", "owner"}, {"wrong_ws", "org", "other_ws", "public", "owner"}} {
		if err := db.Create(&model.Template{ID: f.id, OrganizationID: f.org, Scope: "acl", Name: f.id, PromptBody: "original", OutputAssetKind: "first_frame", Status: "published", Visibility: f.vis, Version: 1, CreatedBy: f.owner, CreatedAt: now, UpdatedAt: now, InvocationMapping: datatypes.JSON(`{}`)}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.Workflow{ID: f.id, OrganizationID: f.org, WorkspaceID: f.ws, Scope: "acl", Name: f.id, OutputStepKey: "result", Status: "draft", Visibility: f.vis, Version: 1, CreatedBy: f.owner, CreatedAt: now, UpdatedAt: now, InputSchema: datatypes.JSON(`[]`)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&model.TemplateGrant{TemplateID: "shared", TeamID: "dept", Permission: "editor", GrantedBy: "owner", GrantedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.WorkflowGrant{WorkflowID: "shared", TeamID: "dept", Permission: "editor", GrantedBy: "owner", GrantedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	a := &boundaryAuthority{facts: shared.ResourceFacts{OrganizationIDs: []string{"org"}, TeamIDs: []string{"dept"}, WorkspaceIDs: []string{"ws"}, WorkflowOrganizationIDs: []string{"org"}}, targetOrg: "org"}
	templates := tplrepo.NewRepo(db)
	templateSvc := tplsvc.NewService(templates, tplrepo.NewGrantRepo(db), tx, nil, nil)
	templateSvc.ConfigureResourceAuthority(a)
	workflowSvc := wfsvc.NewService(wfrepo.NewRepo(db), wfrepo.NewGrantRepo(db), tx, nil, nil)
	workflowSvc.ConfigureResourceAuthority(a)
	ts, total, err := templateSvc.List(ctx, tpldomain.ListQuery{Limit: 1, Offset: 1}, "member")
	if err != nil || total != 4 || len(ts) != 1 {
		t.Fatal("template boundary/count not applied before pagination", err, total)
	}
	ws, total, err := workflowSvc.List(ctx, wfdomain.ListQuery{Limit: 1, Offset: 1}, "member")
	if err != nil || total != 3 || len(ws) != 1 {
		t.Fatal("workflow boundary/count not applied before pagination", err, total)
	}
	a.targetOrg = "other"
	if _, err := templateSvc.Grant(ctx, "shared", "foreign_dept", "viewer", "owner"); !errors.Is(err, tpldomain.ErrTemplateForbidden) {
		t.Fatal("cross enterprise grant persisted", err)
	}
	a.targetOrg = "org"
	if _, err := templateSvc.Revoke(ctx, "shared", "dept", "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := templateSvc.Archive(ctx, "shared", "member", "dept"); !errors.Is(err, tpldomain.ErrTemplateForbidden) {
		t.Fatal("revoked grant archived resource", err)
	}
	// Version publication facts remain independent of the mutable current head.
	if err := db.Create(&model.TemplateVersion{ID: "original_version", TemplateID: "public", Version: 1, Status: "published", PromptBody: "original", OutputAssetKind: "first_frame", InputsSnapshot: datatypes.JSON(`[]`), InvocationMapping: datatypes.JSON(`{}`), CreatedBy: "owner", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	current, err := templates.Get(ctx, "public")
	if err != nil {
		t.Fatal(err)
	}
	current.Version = 2
	current.PromptBody = "new draft"
	current.Status = tpldomain.StatusDraft
	if err := tx.RunInTx(ctx, func(txCtx context.Context) error { return templates.Update(txCtx, current, nil) }); err != nil {
		t.Fatal(err)
	}
	old, err := templates.GetVersion(ctx, "public", 1)
	if err != nil || old.Status != tpldomain.StatusPublished || old.PromptBody != "original" {
		t.Fatal("old published snapshot changed", err)
	}
	next, err := templates.GetVersion(ctx, "public", 2)
	if err != nil || next.Status != tpldomain.StatusDraft {
		t.Fatal("editor implicitly published new version", err)
	}
	if err := tx.RunInTx(ctx, func(txCtx context.Context) error {
		return templates.UpdateStatus(txCtx, "public", tpldomain.StatusPublished)
	}); err != nil {
		t.Fatal(err)
	}
	next, err = templates.GetVersion(ctx, "public", 2)
	if err != nil || next.Status != tpldomain.StatusPublished {
		t.Fatal("publication fact not committed", err)
	}
	// Attempted rebinding is rejected at the persistence boundary as well.
	if err := db.Model(&model.Template{}).Where("id=?", "public").Update("organization_id", "other").Error; err == nil {
		t.Fatal("enterprise was rebound")
	}
}
