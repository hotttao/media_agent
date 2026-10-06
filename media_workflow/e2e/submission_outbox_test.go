//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"media_agent/media_workflow/biz/dal"
	repo "media_agent/media_workflow/biz/dal/repo/job"
	domain "media_agent/media_workflow/biz/domain/job"
)

func TestSubmissionAcceptancePersistence(t *testing.T) {
	db := newDatabase(t)
	store := repo.NewJobRepo(db)
	runner := dal.NewTxRunner(db)
	ctx := context.Background()
	key := domain.SubmissionKey{SubmittedBy: "owner", OrganizationID: "org", WorkspaceID: "workspace", IdempotencyKey: "submit:one"}
	if _, err := store.GetSubmission(ctx, key, true); err == nil {
		t.Fatal("absent key lock permitted outside transaction")
	}
	digest := strings.Repeat("a", 64)
	accept := func(key domain.SubmissionKey, digest, id string, rollback bool) (*domain.Submission, error) {
		var accepted *domain.Submission
		err := runner.RunInTx(ctx, func(tx context.Context) error {
			previous, err := store.GetSubmission(tx, key, true)
			if err != nil {
				return err
			}
			if previous != nil {
				if previous.RequestDigest != digest {
					return domain.ErrSubmissionConflict
				}
				accepted = previous
				return nil
			}
			j := &domain.Job{ID: id, SubmittedBy: key.SubmittedBy, OrganizationID: key.OrganizationID, WorkspaceID: key.WorkspaceID, WorkflowID: "workflow", WorkflowVersion: 1, Scope: "e2e", Status: domain.JobPending}
			sr := &domain.StepRun{ID: "step_" + id, JobID: id, StepKey: "image", TemplateID: "template", TemplateVersion: 1, Status: domain.SRBlocked, ApprovalPolicy: domain.ApprovalWhenMultipleCandidates}
			if err := store.Save(tx, j, []*domain.StepRun{sr}); err != nil {
				return err
			}
			input, _ := json.Marshal(map[string]any{"job_id": id})
			start := domain.ExecutionStart{ID: "start_" + id, JobID: id, WorkflowName: "compiled", WorkflowVersion: 1, CompilationHash: strings.Repeat("b", 64), CorrelationID: id, WorkflowInput: input, TaskDomains: []byte(`{}`)}
			start.PayloadDigest, _ = start.Digest()
			if err := store.EnqueueStart(tx, start); err != nil {
				return err
			}
			receipt := domain.Submission{SubmissionKey: key, RequestDigest: digest, JobID: id}
			if err := store.SaveSubmission(tx, receipt); err != nil {
				return err
			}
			if rollback {
				return errors.New("injected isolated rollback")
			}
			accepted = &receipt
			return nil
		})
		return accepted, err
	}
	if _, err := accept(key, digest, "rollback", true); err == nil {
		t.Fatal("rollback did not fail")
	}
	for _, table := range []string{"jobs", "step_runs", "execution_starts", "job_submissions"} {
		var count int64
		if db.Table("media_workflow."+table).Count(&count).Error != nil || count != 0 {
			t.Fatal("rollback left partial aggregate", table)
		}
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []*domain.Submission
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			accepted, err := accept(key, digest, fmt.Sprintf("job_concurrent_%d", i), false)
			if err != nil {
				t.Error("concurrent acceptance failed")
				return
			}
			mu.Lock()
			results = append(results, accepted)
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	if len(results) != 8 {
		t.Fatal("missing concurrency results")
	}
	for _, r := range results {
		if r.JobID != results[0].JobID {
			t.Fatal("same intent created distinct Jobs")
		}
	}
	for _, table := range []string{"jobs", "step_runs", "execution_starts", "job_submissions"} {
		var count int64
		if db.Table("media_workflow."+table).Count(&count).Error != nil || count != 1 {
			t.Fatal("duplicate aggregate persisted", table)
		}
	}
	if _, err := accept(key, strings.Repeat("c", 64), "conflicting", false); !errors.Is(err, domain.ErrSubmissionConflict) {
		t.Fatal("same key changed semantic input")
	}
	if err := store.UpdateStatus(ctx, results[0].JobID, domain.JobCancelled, "isolated fixture"); err != nil {
		t.Fatal("terminal fixture failed")
	}
	if replay, err := accept(key, digest, "terminal_duplicate", false); err != nil || replay.JobID != results[0].JobID {
		t.Fatal("terminal receipt was not replayed")
	}
	if store.SaveSubmission(ctx, *results[0]) == nil {
		t.Fatal("receipt saved without aggregate transaction")
	}
	for i, scope := range []domain.SubmissionKey{
		{SubmittedBy: "other", OrganizationID: "org", WorkspaceID: "workspace", IdempotencyKey: key.IdempotencyKey},
		{SubmittedBy: "owner", OrganizationID: "other", WorkspaceID: "workspace", IdempotencyKey: key.IdempotencyKey},
		{SubmittedBy: "owner", OrganizationID: "org", WorkspaceID: "other", IdempotencyKey: key.IdempotencyKey},
	} {
		if found, err := store.GetSubmission(ctx, scope, false); err != nil || found != nil {
			t.Fatal("foreign scope exposed receipt")
		}
		if accepted, err := accept(scope, digest, fmt.Sprintf("job_scope_%d", i), false); err != nil || accepted.JobID == results[0].JobID {
			t.Fatal("different scope failed independent acceptance")
		}
	}
	if err := runner.RunInTx(ctx, func(tx context.Context) error {
		wrong := *results[0]
		wrong.SubmittedBy = "forged"
		wrong.IdempotencyKey = "different"
		return store.SaveSubmission(tx, wrong)
	}); !errors.Is(err, domain.ErrSubmissionConflict) {
		t.Fatal("receipt rebound foreign owner's Job")
	}
	down, err := os.ReadFile("../migrations/000019_job_submissions.down.sql")
	if err != nil {
		t.Fatal("downgrade fixture unavailable")
	}
	if db.Exec(string(down)).Error == nil {
		t.Fatal("downgrade discarded accepted submission audit")
	}
}
