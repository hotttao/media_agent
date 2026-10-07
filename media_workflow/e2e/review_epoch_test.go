//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"media_agent/media_workflow/biz/dal"
	repo "media_agent/media_workflow/biz/dal/repo/job"
	domain "media_agent/media_workflow/biz/domain/job"
	service "media_agent/media_workflow/biz/service/job"
	"media_agent/media_workflow/biz/shared/taskref"
)

// Real acceptance transaction and projections, controlled engine snapshots.
// AppendRetryHistory below models only the committed local boundary, not rerun.
func TestReviewRetryEpochPersistence(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	jobs, steps := repo.NewJobRepo(db), repo.NewStepRunRepo(db)
	j := &domain.Job{ID: "job-review-epoch", WorkflowID: "workflow", WorkflowVersion: 1, Scope: "e2e", SubmittedBy: "owner", Status: domain.JobRunning, ConductorWorkflowID: "execution"}
	sr := &domain.StepRun{ID: "step-review-epoch", JobID: j.ID, StepKey: "image", TemplateID: "template", TemplateVersion: 1, Status: domain.SRNeedConfirm}
	if jobs.Save(ctx, j, []*domain.StepRun{sr}) != nil || jobs.UpdateConductorExecution(ctx, j.ID, "execution", "frozen") != nil {
		t.Fatal("epoch seed failed")
	}
	candidates := []domain.Candidate{{Content: "accepted content"}}
	if steps.UpdateCandidates(ctx, sr.ID, candidates) != nil {
		t.Fatal("candidate seed failed")
	}
	makeReview := func(id string, epoch int) domain.ReviewDecision {
		index := 0
		frozen, _ := json.Marshal(candidates)
		output, _ := json.Marshal(map[string]any{"decision": "accept", "selected_index": 0, "selected_candidate": candidates[0]})
		r := domain.ReviewDecision{ID: id, JobID: j.ID, StepRunID: sr.ID, ConductorWorkflowID: "execution", ConductorTaskID: id + "-human", TaskReferenceName: taskref.Reference("image", "approval") + "__2", Generation: 2,
			AcceptedRetryCount: &epoch, Decision: "accept", DecidedBy: "owner", IdempotencyKey: id, SelectedIndex: &index, CandidatesSnapshot: frozen, OutputJSON: output}
		r.RequestDigest, _ = r.Digest()
		return r
	}
	accept := func(r domain.ReviewDecision) (*domain.ReviewDecision, error) {
		var accepted *domain.ReviewDecision
		err := dal.NewTxRunner(db).RunInTx(ctx, func(tx context.Context) error { var err error; accepted, _, err = jobs.AcceptReview(tx, r); return err })
		return accepted, err
	}
	old := makeReview("review-old", 0)
	if _, err := accept(old); err != nil {
		t.Fatal("initial epoch acceptance failed")
	}
	if jobs.AppendRetryHistory(ctx, j.ID, domain.RetryRecord{RetryAt: time.Now(), Reason: "isolated committed cycle"}, 1) != nil {
		t.Fatal("retry boundary failed")
	}
	engine := projectionEngine{execution: &domain.EngineExecution{Status: "RUNNING", Tasks: []domain.EngineTask{{TaskID: "new-render", ReferenceTaskName: taskref.Reference("image", "render") + "__2", Status: "COMPLETED", Output: map[string]any{"prompt": "new retry cycle"}}}}}
	svc := service.NewService(jobs, steps, dal.NewTxRunner(db), nil, nil, &projectionAssets{}, nil, nil, engine)
	svc.ConfigureReviews(jobs)
	if _, _, err := svc.Get(ctx, j.ID); err != nil {
		t.Fatal("new cycle projection failed")
	}
	persisted, err := steps.Get(ctx, sr.ID)
	if err != nil || persisted.RenderedPrompt != "new retry cycle" {
		t.Fatal("old approval blocked same numeric generation in new cycle")
	}
	replay, err := accept(old)
	if err != nil || replay.AcceptedRetryCount == nil || *replay.AcceptedRetryCount != 0 || replay.RequestDigest != old.RequestDigest {
		t.Fatal("retry rebound explicit old decision replay")
	}
	if steps.UpdateStatus(ctx, sr.ID, domain.SRNeedConfirm, nil) != nil {
		t.Fatal("new approval wait seed failed")
	}
	duplicate := makeReview("duplicate-task", 0)
	duplicate.ConductorTaskID = old.ConductorTaskID
	duplicate.RequestDigest, _ = duplicate.Digest()
	if _, err := accept(duplicate); !errors.Is(err, domain.ErrReviewConflict) {
		t.Fatal("second decision accepted for the same exact native task")
	}
	// The count is immutable audit metadata, not task execution authority.
	fresh := makeReview("review-fresh", 0)
	if _, err := accept(fresh); err != nil {
		t.Fatal("fresh cycle approval rejected")
	}
	reloaded, err := repo.NewJobRepo(db).GetReviewByKey(ctx, sr.ID, fresh.IdempotencyKey)
	if err != nil || reloaded.AcceptedRetryCount == nil || *reloaded.AcceptedRetryCount != 0 || reloaded.Validate() != nil {
		t.Fatal("epoch or digest lost through persistence")
	}
	if db.Exec("UPDATE media_workflow.step_run_reviews SET accepted_retry_count=9 WHERE id=?", fresh.ID).Error == nil {
		t.Fatal("accepted epoch mutable")
	}
	down, err := os.ReadFile("../migrations/000021_review_retry_epoch.down.sql")
	if err != nil || db.Exec(string(down)).Error == nil {
		t.Fatal("downgrade dropped epoch audit")
	}
	var count int64
	if db.Table("media_workflow.step_run_reviews").Count(&count).Error != nil || count != 2 {
		t.Fatal("conflict or downgrade altered audit")
	}
}
