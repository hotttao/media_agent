//go:build e2e

package e2e

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"media_agent/media_workflow/biz/dal"
	repo "media_agent/media_workflow/biz/dal/repo/job"
	domain "media_agent/media_workflow/biz/domain/job"
)

// Real isolated PostgreSQL. No production Job, Worker, asset or engine call.
func TestResultReuseEvidencePersistence(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	jobs := repo.NewJobRepo(db)
	store := repo.NewResultReuseRepo(db)
	boundary := domain.ReuseScope{OrganizationID: "org_e2e", WorkspaceID: "workspace_e2e", OwnerUserID: "owner_e2e", Scope: "media"}
	j := &domain.Job{ID: "job_reuse_e2e", WorkflowID: "workflow", WorkflowVersion: 1, OrganizationID: boundary.OrganizationID, WorkspaceID: boundary.WorkspaceID, SubmittedBy: boundary.OwnerUserID, Scope: boundary.Scope, Status: domain.JobRunning}
	step := &domain.StepRun{ID: "step_reuse_e2e", JobID: j.ID, StepKey: "image", StepOrder: 1, TemplateID: "template", TemplateVersion: 1, Status: domain.SRPending, ApprovalPolicy: domain.ApprovalWhenMultipleCandidates, AssetKind: "model_image"}
	if err := jobs.Save(ctx, j, []*domain.StepRun{step}); err != nil {
		t.Fatal("isolated source seed failed")
	}
	evidence := domain.ReuseEvidence{ID: domain.ReuseEvidenceID(step.ID, "result"), Boundary: boundary, Fingerprint: strings.Repeat("a", 64), SourceJobID: j.ID, SourceStepRunID: step.ID, SourceResultKey: "result", AssetID: "gen_fixture", ObjectPath: "materials/org_e2e/media/fixture.png", AssetKind: step.AssetKind}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := store.Save(ctx, evidence); err != nil {
				t.Error("identical concurrent evidence did not replay")
			}
		}()
	}
	wg.Wait()
	found, err := store.Find(ctx, boundary, evidence.Fingerprint)
	if err != nil || len(found) != 1 || !found[0].Same(evidence) {
		t.Fatal("reuse evidence was not persisted once")
	}
	createdAt := found[0].CreatedAt
	if db.Exec("UPDATE media_workflow.step_result_reuse SET asset_id='gen_changed' WHERE id=?", evidence.ID).Error == nil {
		t.Fatal("database allowed accepted evidence to change")
	}
	changed := evidence
	changed.AssetID = "gen_other"
	if !errors.Is(store.Save(ctx, changed), domain.ErrReuseEvidenceConflict) {
		t.Fatal("different result reused the original evidence key")
	}
	for _, boundary := range []domain.ReuseScope{
		{OrganizationID: "other", WorkspaceID: "workspace_e2e", OwnerUserID: "owner_e2e", Scope: "media"},
		{OrganizationID: "org_e2e", WorkspaceID: "other", OwnerUserID: "owner_e2e", Scope: "media"},
		{OrganizationID: "org_e2e", WorkspaceID: "workspace_e2e", OwnerUserID: "other", Scope: "media"},
		{OrganizationID: "org_e2e", WorkspaceID: "workspace_e2e", OwnerUserID: "owner_e2e", Scope: "other"},
	} {
		found, err := store.Find(ctx, boundary, evidence.Fingerprint)
		if err != nil || len(found) != 0 {
			t.Fatal("reuse evidence crossed an execution boundary")
		}
	}
	forged := evidence
	forged.Boundary.OwnerUserID = "other"
	forged.SourceResultKey = "forged"
	forged.ID = domain.ReuseEvidenceID(step.ID, forged.SourceResultKey)
	if !errors.Is(store.Save(ctx, forged), domain.ErrReuseEvidenceConflict) {
		t.Fatal("source ownership was not checked")
	}
	rolled := evidence
	rolled.SourceResultKey = "rollback"
	rolled.ID = domain.ReuseEvidenceID(step.ID, rolled.SourceResultKey)
	stop := errors.New("test rollback")
	if err := dal.NewTxRunner(db).RunInTx(ctx, func(tx context.Context) error {
		if err := store.Save(tx, rolled); err != nil {
			return err
		}
		return stop
	}); !errors.Is(err, stop) {
		t.Fatal("isolated evidence rollback failed")
	}
	found, err = store.Find(ctx, boundary, evidence.Fingerprint)
	if err != nil || len(found) != 1 || !found[0].CreatedAt.Equal(createdAt) {
		t.Fatal("rollback or replay modified the original evidence")
	}
}
