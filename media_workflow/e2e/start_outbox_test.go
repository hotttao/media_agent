//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"media_agent/media_workflow/biz/dal"
	repo "media_agent/media_workflow/biz/dal/repo/job"
	domain "media_agent/media_workflow/biz/domain/job"
)

// Isolated PostgreSQL protocol test; no runtime Submit, Worker or Conductor.
func TestExecutionStartOutboxPersistence(t *testing.T) {
	db := newDatabase(t)
	store := repo.NewJobRepo(db)
	runner := dal.NewTxRunner(db)
	ctx := context.Background()
	seed := func(name string, rollback bool) domain.ExecutionStart {
		t.Helper()
		j := &domain.Job{ID: "job_" + name, Scope: "e2e", WorkflowID: "workflow", WorkflowVersion: 1, Status: domain.JobPending, SubmittedBy: "isolated-owner", SubmittedAt: time.Now()}
		sr := &domain.StepRun{ID: "step_" + name, JobID: j.ID, StepKey: "image", StepOrder: 1, TemplateID: "template", TemplateVersion: 1, Status: domain.SRBlocked, ApprovalPolicy: domain.ApprovalWhenMultipleCandidates}
		input, _ := json.Marshal(map[string]any{"job_id": j.ID, "inputs": map[string]any{}})
		s := domain.ExecutionStart{ID: "start_" + name, JobID: j.ID, WorkflowName: "compiled", WorkflowVersion: 1, CompilationHash: strings.Repeat("a", 64), CorrelationID: j.ID, WorkflowInput: input, TaskDomains: json.RawMessage(`{}`)}
		s.PayloadDigest, _ = s.Digest()
		stop := errors.New("isolated rollback")
		err := runner.RunInTx(ctx, func(tx context.Context) error {
			if err := store.Save(tx, j, []*domain.StepRun{sr}); err != nil {
				return err
			}
			if err := store.EnqueueStart(tx, s); err != nil {
				return err
			}
			if rollback {
				return stop
			}
			return nil
		})
		if (rollback && !errors.Is(err, stop)) || (!rollback && err != nil) {
			t.Fatal("isolated atomic seed failed")
		}
		return s
	}
	rolled := seed("rollback", true)
	for _, table := range []string{"jobs", "step_runs", "execution_starts"} {
		var count int64
		if err := db.Table("media_workflow." + table).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("atomic rollback retained partial aggregate")
		}
	}
	if store.EnqueueStart(ctx, rolled) == nil {
		t.Fatal("start persisted without caller transaction")
	}
	frozen := seed("first", false)
	duplicate := frozen
	duplicate.ID = "different-start-id"
	if err := runner.RunInTx(ctx, func(tx context.Context) error { return store.EnqueueStart(tx, duplicate) }); err == nil {
		t.Fatal("one native Job accepted a second initial start")
	}
	for _, mutation := range []string{
		"workflow_input='{}'::jsonb",
		"status='started'",
		"claim_token='unpaired-token'",
	} {
		if db.Exec("UPDATE media_workflow.execution_starts SET "+mutation+" WHERE id=?", frozen.ID).Error == nil {
			t.Fatal("database accepted an invalid startup invariant")
		}
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	claims := []*domain.ExecutionStart{}
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, err := store.ClaimStart(ctx)
			if err != nil {
				t.Error("concurrent claim failed")
				return
			}
			if claim != nil {
				mu.Lock()
				claims = append(claims, claim)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(claims) != 1 || claims[0].ClaimToken == "" || claims[0].Status != "queued" {
		t.Fatal("concurrent claim did not produce one fenced owner")
	}
	old := *claims[0]
	if store.FinishStart(ctx, old, "execution") == nil {
		t.Fatal("unsent start was marked completed")
	}
	if err := store.MarkStartSent(ctx, old); err != nil {
		t.Fatal("sent transition failed")
	}
	if store.MarkStartSent(ctx, old) == nil {
		t.Fatal("uncertain command authorized another send")
	}
	if err := db.Exec("UPDATE media_workflow.execution_starts SET claimed_at=now()-interval '3 minutes' WHERE id=?", old.ID).Error; err != nil {
		t.Fatal("isolated lease setup failed")
	}
	if store.FinishStart(ctx, old, "late") == nil {
		t.Fatal("expired owner completed start")
	}
	current, err := store.ClaimStart(ctx)
	if err != nil || current == nil || current.Status != "uncertain" || current.ClaimToken == old.ClaimToken {
		t.Fatal("uncertain claim not recovered with new fencing")
	}
	if store.MarkStartSent(ctx, *current) == nil {
		t.Fatal("recovered uncertain authorized another send")
	}
	if store.FinishStart(ctx, old, "stale") == nil || store.ReleaseStartClaim(ctx, old, "unavailable") == nil {
		t.Fatal("stale claim changed newer owner")
	}
	if store.ReleaseStartClaim(ctx, *current, "https://secret.example?token=private") == nil {
		t.Fatal("unsafe error stored")
	}
	if err := store.ReleaseStartClaim(ctx, *current, "execution_unavailable"); err != nil {
		t.Fatal("claim release failed")
	}
	if next, err := store.ClaimStart(ctx); err != nil || next != nil {
		t.Fatal("release bypassed backoff")
	}
	if err := db.Exec("UPDATE media_workflow.execution_starts SET next_attempt_at=now()-interval '1 second' WHERE id=?", old.ID).Error; err != nil {
		t.Fatal("isolated backoff setup failed")
	}
	current, err = store.ClaimStart(ctx)
	if err != nil || current == nil || current.Status != "uncertain" {
		t.Fatal("release reverted uncertain command to queued")
	}
	wrong := *current
	wrong.PayloadDigest = strings.Repeat("b", 64)
	if store.FinishStart(ctx, wrong, "wrong-execution") == nil {
		t.Fatal("completion ignored frozen digest")
	}
	pending, err := store.Get(ctx, current.JobID)
	if err != nil || pending.Status != domain.JobPending || pending.ConductorWorkflowID != "" {
		t.Fatal("failed completion partially bound Job")
	}
	if err := store.FinishStart(ctx, *current, "confirmed-execution"); err != nil {
		t.Fatal("atomic instance binding failed")
	}
	job, err := store.Get(ctx, current.JobID)
	if err != nil || job.Status != domain.JobRunning || job.ConductorWorkflowID != "confirmed-execution" || job.CompilationHash != current.CompilationHash {
		t.Fatal("Job was not atomically bound")
	}
	if next, err := store.ClaimStart(ctx); err != nil || next != nil {
		t.Fatal("started command was claimed again")
	}
	seed("cancelled", false)
	cancelled, err := store.ClaimStart(ctx)
	if err != nil || cancelled == nil {
		t.Fatal("cancel fixture not claimed")
	}
	if handled, err := store.CancelInitialStart(ctx, cancelled.JobID); err != nil || !handled {
		t.Fatal("cancel fixture failed")
	}
	if store.MarkStartSent(ctx, *cancelled) == nil || store.FinishStart(ctx, *cancelled, "forbidden") == nil {
		t.Fatal("cancelled Job was revived")
	}
	if handled, err := store.CancelInitialStart(ctx, cancelled.JobID); err != nil || !handled {
		t.Fatal("cancel replay failed")
	}
	if next, err := store.ClaimStart(ctx); err != nil || next != nil {
		t.Fatal("unsent cancelled command was reclaimed")
	}

	seed("cancel_unknown", false)
	unknown, err := store.ClaimStart(ctx)
	if err != nil || unknown == nil {
		t.Fatal("unknown fixture claim failed")
	}
	if err := store.MarkStartSent(ctx, *unknown); err != nil {
		t.Fatal("unknown fixture sent mark failed")
	}
	if handled, err := store.CancelInitialStart(ctx, unknown.JobID); err != nil || !handled {
		t.Fatal("uncertain cancellation failed")
	}
	if store.FinishStart(ctx, *unknown, "late-running") == nil {
		t.Fatal("in-flight completion revived cancelled Job")
	}
	bad := *unknown
	bad.ClaimToken = "stale"
	if store.FinishCancelledStart(ctx, bad, "terminated-execution") == nil {
		t.Fatal("stale claim completed cleanup")
	}
	if err := store.FinishCancelledStart(ctx, *unknown, "terminated-execution"); err != nil {
		t.Fatal("uncertain cleanup audit failed")
	}
	var unknownState struct{ Status, ConductorWorkflowID string }
	if err := db.Table("media_workflow.execution_starts").Where("id=?", unknown.ID).Take(&unknownState).Error; err != nil || unknownState.Status != "cancelled" || unknownState.ConductorWorkflowID != "terminated-execution" {
		t.Fatal("cleanup audit missing")
	}
	j, err := store.Get(ctx, unknown.JobID)
	if err != nil || j.Status != domain.JobCancelled || j.ConductorWorkflowID != "" {
		t.Fatal("cleanup rebound Job")
	}

	if handled, err := store.CancelInitialStart(ctx, current.JobID); err != nil || !handled {
		t.Fatal("bound instance cancellation failed")
	}
	cleanup, err := store.ClaimStart(ctx)
	if err != nil || cleanup == nil || cleanup.Status != "cancel_pending" || cleanup.ConductorWorkflowID != "confirmed-execution" {
		t.Fatal("bound cancellation lost cleanup obligation")
	}
	if store.FinishCancelledStart(ctx, *cleanup, "different-instance") == nil {
		t.Fatal("cleanup replaced bound execution")
	}
	if err := store.FinishCancelledStart(ctx, *cleanup, "confirmed-execution"); err != nil {
		t.Fatal("bound cleanup failed")
	}
	if next, err := store.ClaimStart(ctx); err != nil || next != nil {
		t.Fatal("finished cleanup was reclaimed")
	}
	downgradeCancel, err := os.ReadFile("../migrations/000018_execution_start_cancellation.down.sql")
	if err != nil {
		t.Fatal("cancellation downgrade fixture unavailable")
	}
	if db.Exec(string(downgradeCancel)).Error == nil {
		t.Fatal("downgrade discarded cancellation audit")
	}
	down, err := os.ReadFile("../migrations/000017_execution_starts.down.sql")
	if err != nil {
		t.Fatal("downgrade fixture unavailable")
	}
	if db.Exec(string(down)).Error == nil {
		t.Fatal("downgrade deleted startup audit")
	}
}
