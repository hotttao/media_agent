//go:build e2e

package e2e

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"media_agent/media_workflow/biz/dal"
	repo "media_agent/media_workflow/biz/dal/repo/workflow"
	domain "media_agent/media_workflow/biz/domain/workflow"
)

// Real repository and PostgreSQL, no Conductor or publication HTTP calls.
func TestCompilationFrozenPersistence(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	workflows := repo.NewRepo(db)
	if err := workflows.Save(ctx, &domain.Workflow{ID: "wf-frozen", Scope: "e2e", Name: "frozen", OutputStepKey: "image", Status: domain.StatusDraft, Visibility: domain.VisibilityPrivate, Version: 1, CreatedBy: "owner"}); err != nil {
		t.Fatal("isolated workflow seed failed")
	}
	store := repo.NewCompilationRepo(db)
	base := domain.Compilation{ID: "comp-frozen", WorkflowID: "wf-frozen", WorkflowVersion: 1, ConductorName: "frozen", ConductorVersion: 1,
		IRJSON: []byte(`{"nodes":["image"],"version":1}`), DefinitionJSON: []byte(`{"name":"frozen","version":1,"tasks":[]}`), DefinitionHash: strings.Repeat("a", 64), Status: "registering", CreatedAt: time.Now()}
	if err := dal.NewTxRunner(db).RunInTx(ctx, func(tx context.Context) error {
		if err := store.Save(tx, &base); err != nil {
			return err
		}
		return errors.New("injected rollback")
	}); err == nil {
		t.Fatal("injected rollback ignored")
	}
	var count int64
	if db.Table("media_workflow.workflow_compilations").Count(&count).Error != nil || count != 0 {
		t.Fatal("compilation escaped caller rollback")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); copy := base; errs <- store.Save(ctx, &copy) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("same compilation concurrent replay failed")
		}
	}
	if db.Table("media_workflow.workflow_compilations").Count(&count).Error != nil || count != 1 {
		t.Fatal("compilation duplicated")
	}
	if err := store.MarkRegistered(ctx, base.ID); err != nil {
		t.Fatal("registration mark failed")
	}
	first, err := store.GetRegistered(ctx, base.WorkflowID, base.WorkflowVersion)
	if err != nil || first.RegisteredAt == nil {
		t.Fatal("registered compilation unavailable")
	}
	replay := base
	replay.IRJSON = []byte(`{ "version":1, "nodes":["image"] }`)
	replay.DefinitionJSON = []byte(`{"tasks":[],"version":1,"name":"frozen"}`)
	replay.CreatedAt = time.Now().Add(time.Hour)
	replay.ErrorMessage = "replayed failure must not reset lifecycle"
	if store.Save(ctx, &replay) != nil || store.MarkFailed(ctx, base.ID, "late_failure") != nil || store.MarkRegistered(ctx, base.ID) != nil {
		t.Fatal("same-content lifecycle replay failed")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*domain.Compilation)
	}{
		{"identity", func(c *domain.Compilation) { c.ID = "other-id" }},
		{"IR", func(c *domain.Compilation) { c.IRJSON = []byte(`{"nodes":["other"]}`) }},
		{"definition", func(c *domain.Compilation) { c.DefinitionJSON = []byte(`{"name":"changed"}`) }},
		{"hash", func(c *domain.Compilation) { c.DefinitionHash = strings.Repeat("b", 64) }},
		{"engine name", func(c *domain.Compilation) { c.ConductorName = "changed" }},
		{"engine version", func(c *domain.Compilation) { c.ConductorVersion = 2 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := base
			tc.mutate(&changed)
			if err := store.Save(ctx, &changed); !errors.Is(err, domain.ErrCompilationConflict) {
				t.Fatal("frozen version accepted changed content")
			}
		})
	}
	final, err := repo.NewCompilationRepo(db).GetRegistered(ctx, base.WorkflowID, 1)
	if err != nil || final.ID != first.ID || final.ErrorMessage != "" || final.DefinitionHash != first.DefinitionHash || !final.RegisteredAt.Equal(*first.RegisteredAt) || !final.CreatedAt.Equal(first.CreatedAt) {
		t.Fatal("replay or late failure changed registered compilation")
	}
	if !errors.Is(store.MarkFailed(ctx, "missing", "failure"), gorm.ErrRecordNotFound) || !errors.Is(store.MarkRegistered(ctx, "missing"), gorm.ErrRecordNotFound) {
		t.Fatal("missing compilation mark reported success")
	}
	for _, update := range []string{
		"UPDATE media_workflow.workflow_compilations SET ir_json='{}' WHERE id='comp-frozen'",
		"UPDATE media_workflow.workflow_compilations SET status='failed' WHERE id='comp-frozen'",
		"UPDATE media_workflow.workflow_compilations SET registered_at=now()+interval '1 hour' WHERE id='comp-frozen'",
	} {
		if db.Exec(update).Error == nil {
			t.Fatal("database allowed frozen compilation mutation")
		}
	}
	// A new business version can hold a different immutable compilation.
	next := base
	next.ID = "comp-frozen-2"
	next.WorkflowVersion = 2
	next.ConductorVersion = 2
	next.DefinitionHash = strings.Repeat("b", 64)
	if store.Save(ctx, &next) != nil {
		t.Fatal("new version compilation rejected")
	}
	if db.Table("media_workflow.workflow_compilations").Count(&count).Error != nil || count != 2 {
		t.Fatal("new version did not create independent compilation")
	}
	if store.MarkFailed(ctx, next.ID, "temporary_registration_failure") != nil || store.Save(ctx, &next) != nil {
		t.Fatal("failed registration replay rejected")
	}
	var status string
	if db.Table("media_workflow.workflow_compilations").Select("status").Where("id=?", next.ID).Scan(&status).Error != nil || status != "failed" {
		t.Fatal("Save reset failed lifecycle instead of replaying frozen content")
	}
	if store.MarkRegistered(ctx, next.ID) != nil {
		t.Fatal("exact frozen compilation could not recover from registration failure")
	}
}
