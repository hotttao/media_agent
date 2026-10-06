//go:build e2e

package e2e

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"media_agent/media_workflow/biz/dal"
	repo "media_agent/media_workflow/biz/dal/repo/workflow"
	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/compiler"
)

func TestPublicationEditingVersionGuard(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	workflows, intents, tx := repo.NewRepo(db), repo.NewCompilationRepo(db), dal.NewTxRunner(db)
	w := &domain.Workflow{ID: "wf_edit", Name: "source", Scope: "e2e", Version: 1, CreatedBy: "owner", Status: domain.StatusDraft, Visibility: domain.VisibilityPrivate, OutputStepKey: "image"}
	if workflows.Save(ctx, w) != nil {
		t.Fatal("source seed failed")
	}
	definition := compiler.Definition{Name: "wf_edit", Version: 1, SchemaVersion: 2}
	encoded, _ := compiler.CanonicalJSON(definition)
	hash, _ := compiler.DefinitionHash(definition)
	c := &domain.Compilation{ID: "comp_edit", WorkflowID: w.ID, WorkflowVersion: 1, ConductorName: "wf_edit", ConductorVersion: 1, IRJSON: []byte(`{"steps":[]}`), DefinitionJSON: encoded, DefinitionHash: hash, Status: "registering", CreatedAt: time.Now()}
	p := domain.Publication{ID: "pub_edit", WorkflowID: w.ID, WorkflowVersion: 1, CompilationID: c.ID, RequestedBy: "owner", DefinitionHash: hash, DefinitionJSON: encoded, TaskDefinitionsJSON: []byte(`[{"name":"render"}]`)}
	p.PayloadDigest, _ = p.Digest()
	if tx.RunInTx(ctx, func(ctx context.Context) error { _, _, err := intents.EnqueuePublication(ctx, c, p); return err }) != nil {
		t.Fatal("publication acceptance failed")
	}

	var wg sync.WaitGroup
	errorsOut := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			edit := *w
			edit.Version, edit.Name = 2, "new draft"
			errorsOut <- tx.RunInTx(ctx, func(ctx context.Context) error { return workflows.Update(ctx, &edit) })
		}()
	}
	wg.Wait()
	close(errorsOut)
	winners := 0
	for err := range errorsOut {
		if err == nil {
			winners++
		} else if !errors.Is(err, domain.ErrPublicationConflict) {
			t.Fatal("unexpected concurrent edit error")
		}
	}
	if winners != 1 {
		t.Fatal("lost-update guard did not select exactly one edit")
	}
	current, err := workflows.Get(ctx, w.ID)
	if err != nil || current.Version != 2 || current.Status != domain.StatusDraft {
		t.Fatal("new version was not draft")
	}
	lease, err := intents.ClaimPublication(ctx)
	if err != nil || lease == nil {
		t.Fatal("old intent not claimable")
	}
	if tx.RunInTx(ctx, func(ctx context.Context) error { return intents.FinishPublication(ctx, *lease, "registered", "") }) != nil {
		t.Fatal("old intent supersede failed")
	}
	prior, _ := intents.GetPublication(ctx, w.ID, 1)
	current, _ = workflows.Get(ctx, w.ID)
	if prior.Status != "superseded" || current.Status != domain.StatusDraft || current.Version != 2 {
		t.Fatal("old publication modified new source")
	}

	badOwner := *current
	badOwner.Version, badOwner.CreatedBy = 3, "other"
	if err := tx.RunInTx(ctx, func(ctx context.Context) error { return workflows.Update(ctx, &badOwner) }); !errors.Is(err, domain.ErrPublicationConflict) {
		t.Fatal("owner change bypassed edit guard")
	}
	if workflows.UpdateStatus(ctx, w.ID, domain.StatusArchived) != nil {
		t.Fatal("archive failed")
	}
	edit := *current
	edit.Version = 3
	if err := tx.RunInTx(ctx, func(ctx context.Context) error { return workflows.Update(ctx, &edit) }); !errors.Is(err, domain.ErrPublicationConflict) {
		t.Fatal("archive resurrected by stale edit")
	}
}
