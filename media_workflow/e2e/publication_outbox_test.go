//go:build e2e

package e2e

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"media_agent/media_workflow/biz/dal"
	repo "media_agent/media_workflow/biz/dal/repo/workflow"
	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/compiler"
)

// Isolated journal contract, no compiler service, auth or Conductor registration.
func TestPublicationOutboxPersistence(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	workflows, store, tx := repo.NewRepo(db), repo.NewCompilationRepo(db), dal.NewTxRunner(db)
	seed := func(id string) (*domain.Compilation, domain.Publication) {
		t.Helper()
		if workflows.Save(ctx, &domain.Workflow{ID: id, Name: id, Scope: "e2e", Version: 1, Status: domain.StatusDraft, Visibility: domain.VisibilityPrivate, CreatedBy: "owner", OutputStepKey: "image"}) != nil {
			t.Fatal("publication source seed failed")
		}
		definition := compiler.Definition{Name: id, Version: 1, SchemaVersion: 2, Tasks: []compiler.Task{{Name: "render", TaskReferenceName: "render", Type: "SIMPLE"}}}
		encoded, _ := compiler.CanonicalJSON(definition)
		hash, _ := compiler.DefinitionHash(definition)
		c := &domain.Compilation{ID: "comp_" + id, WorkflowID: id, WorkflowVersion: 1, ConductorName: id, ConductorVersion: 1, IRJSON: []byte(`{"steps":[]}`), DefinitionJSON: encoded, DefinitionHash: hash, Status: "registering", CreatedAt: time.Now()}
		p := domain.Publication{ID: "pub_" + id, WorkflowID: id, WorkflowVersion: 1, CompilationID: c.ID, RequestedBy: "owner", DefinitionHash: hash, DefinitionJSON: encoded, TaskDefinitionsJSON: []byte(`[{"name":"render","retryCount":2}]`)}
		p.PayloadDigest, _ = p.Digest()
		return c, p
	}
	accept := func(c *domain.Compilation, p domain.Publication, rollback bool) (*domain.Publication, error) {
		var accepted *domain.Publication
		err := tx.RunInTx(ctx, func(txCtx context.Context) error {
			var err error
			accepted, _, err = store.EnqueuePublication(txCtx, c, p)
			if err != nil {
				return err
			}
			if rollback {
				return errors.New("injected rollback")
			}
			return nil
		})
		return accepted, err
	}
	finish := func(p domain.Publication, status string, rollback bool) error {
		return tx.RunInTx(ctx, func(txCtx context.Context) error {
			if err := store.FinishPublication(txCtx, p, status, ""); err != nil {
				return err
			}
			if rollback {
				return errors.New("injected completion rollback")
			}
			return nil
		})
	}
	count := func(table string, want int64) {
		t.Helper()
		var n int64
		if db.Table("media_workflow."+table).Count(&n).Error != nil || n != want {
			t.Fatal("unexpected persisted count")
		}
	}
	c, p := seed("wf_publication")
	if _, _, err := store.EnqueuePublication(ctx, c, p); err == nil {
		t.Fatal("acceptance outside transaction succeeded")
	}
	if _, err := accept(c, p, true); err == nil {
		t.Fatal("rollback ignored")
	}
	count("workflow_compilations", 0)
	count("workflow_publications", 0)
	source, _ := workflows.Get(ctx, p.WorkflowID)
	if source.Status != domain.StatusDraft {
		t.Fatal("rollback left publishing source")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			copy := *c
			accepted, err := accept(&copy, p, false)
			if err == nil && accepted.ID != p.ID {
				err = errors.New("receipt identity changed")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("concurrent publication acceptance failed")
		}
	}
	count("workflow_compilations", 1)
	count("workflow_publications", 1)
	source, _ = workflows.Get(ctx, p.WorkflowID)
	if string(source.Status) != "publishing" {
		t.Fatal("accepted intent did not mark publishing")
	}
	changed := p
	changed.TaskDefinitionsJSON = []byte(`[{"name":"render","retryCount":9}]`)
	changed.PayloadDigest, _ = changed.Digest()
	if _, err := accept(c, changed, false); !errors.Is(err, domain.ErrPublicationConflict) {
		t.Fatal("same version changed registration contract")
	}
	lease, err := store.ClaimPublication(ctx)
	if err != nil || lease == nil || lease.Validate() != nil || lease.ClaimToken == "" {
		t.Fatal("claim failed")
	}
	if other, err := store.ClaimPublication(ctx); err != nil || other != nil {
		t.Fatal("active lease was claimed twice")
	}
	wrong := *lease
	wrong.ClaimToken = "wrong"
	if !errors.Is(store.ReleasePublication(ctx, wrong, "temporary"), domain.ErrPublicationClaimStale) || !errors.Is(finish(wrong, "registered", false), domain.ErrPublicationClaimStale) {
		t.Fatal("wrong claim accepted")
	}
	if store.ReleasePublication(ctx, *lease, "registration_unavailable") != nil {
		t.Fatal("claim release failed")
	}
	if next, err := store.ClaimPublication(ctx); err != nil || next != nil {
		t.Fatal("backoff ignored")
	}
	if db.Exec("UPDATE media_workflow.workflow_publications SET next_attempt_at=now()-interval '1 second' WHERE id=?", p.ID).Error != nil {
		t.Fatal("isolated backoff advancement failed")
	}
	second, err := store.ClaimPublication(ctx)
	if err != nil || second == nil || second.ClaimToken == lease.ClaimToken {
		t.Fatal("fresh claim not fenced")
	}
	if !errors.Is(finish(*lease, "registered", false), domain.ErrPublicationClaimStale) {
		t.Fatal("released process completed later claim")
	}
	if db.Exec("UPDATE media_workflow.workflow_publications SET claimed_at=now()-interval '3 minutes' WHERE id=?", p.ID).Error != nil {
		t.Fatal("isolated lease expiration failed")
	}
	third, err := repo.NewCompilationRepo(db).ClaimPublication(ctx)
	if err != nil || third == nil || third.ClaimToken == second.ClaimToken {
		t.Fatal("new repository could not recover expired claim")
	}
	if !errors.Is(store.ReleasePublication(ctx, *second, "stale"), domain.ErrPublicationClaimStale) {
		t.Fatal("expired process released new claim")
	}
	if finish(*third, "registered", true) == nil {
		t.Fatal("completion rollback ignored")
	}
	current, _ := store.GetPublication(ctx, p.WorkflowID, 1)
	source, _ = workflows.Get(ctx, p.WorkflowID)
	if current.Status != "pending" || current.ClaimToken != third.ClaimToken || string(source.Status) != "publishing" {
		t.Fatal("partial successful completion escaped rollback")
	}
	if finish(*third, "registered", false) != nil {
		t.Fatal("completion failed")
	}
	current, _ = store.GetPublication(ctx, p.WorkflowID, 1)
	source, _ = workflows.Get(ctx, p.WorkflowID)
	if current.Status != "registered" || current.ClaimToken != "" || source.Status != domain.StatusPublished {
		t.Fatal("registration and publication did not commit together")
	}
	if _, err := store.GetRegistered(ctx, p.WorkflowID, 1); err != nil {
		t.Fatal("compilation not registered")
	}
	if replay, err := accept(c, p, false); err != nil || replay.Status != "registered" {
		t.Fatal("terminal intent replay reset publication")
	}
	for _, query := range []string{
		"UPDATE media_workflow.workflow_publications SET task_definitions_json='[]' WHERE id='pub_wf_publication'",
		"UPDATE media_workflow.workflow_publications SET status='pending' WHERE id='pub_wf_publication'",
		"DELETE FROM media_workflow.workflow_publications WHERE id='pub_wf_publication'",
	} {
		if db.Exec(query).Error == nil {
			t.Fatal("immutable intent or terminal audit was modified")
		}
	}
	down, err := os.ReadFile("../migrations/000022_workflow_publications.down.sql")
	if err != nil || db.Exec(string(down)).Error == nil {
		t.Fatal("nonempty publication downgrade succeeded")
	}
	count("workflow_publications", 1)
	for _, mode := range []string{"archived", "new_version", "failed"} {
		c, p := seed("wf_" + mode)
		if _, err := accept(c, p, false); err != nil {
			t.Fatal("additional source acceptance failed")
		}
		claimed, err := store.ClaimPublication(ctx)
		if err != nil || claimed == nil || claimed.ID != p.ID {
			t.Fatal("additional publication claim failed")
		}
		status := "registered"
		switch mode {
		case "archived":
			if workflows.UpdateStatus(ctx, p.WorkflowID, domain.StatusArchived) != nil {
				t.Fatal("archive setup failed")
			}
		case "new_version":
			w, _ := workflows.Get(ctx, p.WorkflowID)
			w.Version++
			if workflows.Update(ctx, w) != nil {
				t.Fatal("new version setup failed")
			}
		case "failed":
			status = "failed"
		}
		if finish(*claimed, status, false) != nil {
			t.Fatal("source reconciliation failed")
		}
		current, _ := store.GetPublication(ctx, p.WorkflowID, 1)
		source, _ = workflows.Get(ctx, p.WorkflowID)
		if mode == "failed" {
			if current.Status != "failed" || string(source.Status) != "publish_failed" {
				t.Fatal("failure state not atomic")
			}
		} else if current.Status != "superseded" || source.Status == domain.StatusPublished {
			t.Fatal("old intent published changed source")
		}
	}
	_ = strings.TrimSpace // Keep errors/payloads out of diagnostics.
}
