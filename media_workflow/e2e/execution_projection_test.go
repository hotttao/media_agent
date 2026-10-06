//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"media_agent/media_workflow/biz/dal"
	repo "media_agent/media_workflow/biz/dal/repo/job"
	domain "media_agent/media_workflow/biz/domain/job"
	service "media_agent/media_workflow/biz/service/job"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/shared/taskref"
)

type projectionEngine struct {
	domain.WorkflowEngine
	execution *domain.EngineExecution
	afterRead func()
}

func (e projectionEngine) Get(context.Context, string) (*domain.EngineExecution, error) {
	if e.afterRead != nil {
		e.afterRead()
	}
	return e.execution, nil
}

type projectionAssets struct {
	calls int
	after func()
}

func (a *projectionAssets) UpsertGeneratedAsset(context.Context, *client.UpsertGenAssetReq) (string, error) {
	a.calls++
	if a.after != nil {
		a.after()
	}
	return "stable-generated-asset", nil
}

type failProjectionStatus struct {
	domain.StepRunRepo
	fail bool
}

func (r *failProjectionStatus) UpdateStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
	if err := r.StepRunRepo.UpdateStatus(ctx, id, status, fields); err != nil {
		return err
	}
	if r.fail {
		return errors.New("injected failure after database write")
	}
	return nil
}

// Real local persistence; engine snapshot and Materials response are controlled
// ports. This is not an HTTP, real Conductor or Materials idempotency proof.
func TestExecutionProjectionAtomicPersistence(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	jobs, steps := repo.NewJobRepo(db), repo.NewStepRunRepo(db)
	seed := func(id string) (*domain.Job, *domain.StepRun) {
		t.Helper()
		j := &domain.Job{ID: "job_" + id, Scope: "e2e", WorkflowID: "workflow", WorkflowVersion: 1, Status: domain.JobRunning, SubmittedBy: "isolated-owner", SubmittedAt: time.Now(), ConductorWorkflowID: "engine_" + id}
		sr := &domain.StepRun{ID: "step_" + id, JobID: j.ID, StepKey: "image", StepOrder: 1, TemplateID: "template", TemplateVersion: 1, Status: domain.SRClaimed, ApprovalPolicy: domain.ApprovalWhenMultipleCandidates}
		if err := jobs.Save(ctx, j, []*domain.StepRun{sr}); err != nil {
			t.Fatal("seed failed")
		}
		if err := jobs.UpdateConductorExecution(ctx, j.ID, j.ConductorWorkflowID, "fixture-compilation"); err != nil {
			t.Fatal("engine binding seed failed")
		}
		return j, sr
	}
	makeService := func(stepRepo domain.StepRunRepo, assets *projectionAssets) *service.Service {
		engine := projectionEngine{execution: &domain.EngineExecution{Status: "COMPLETED", Tasks: []domain.EngineTask{{
			ReferenceTaskName: "step__image__finalize", Status: "COMPLETED", Output: map[string]any{"output_resource": map[string]any{"content": "rendered text"}},
		}, {TaskID: "successful-invoke", ReferenceTaskName: "step__image__invoke", Status: "COMPLETED"}}}}
		svc := service.NewService(jobs, stepRepo, dal.NewTxRunner(db), nil, nil, assets, nil, nil, engine)
		svc.ConfigureReviews(jobs)
		return svc
	}
	j, sr := seed("rollback")
	fault := &failProjectionStatus{StepRunRepo: steps, fail: true}
	assets := &projectionAssets{}
	svc := makeService(fault, assets)
	if _, _, err := svc.Get(ctx, j.ID); err == nil {
		t.Fatal("write failure ignored")
	}
	persisted, err := steps.Get(ctx, sr.ID)
	if err != nil || persisted.Status != domain.SRClaimed || persisted.OutputAssetID != "" {
		t.Fatal("output and status were not rolled back together")
	}
	current, err := jobs.Get(ctx, j.ID)
	if err != nil || current.Status != domain.JobRunning {
		t.Fatal("failed projection completed job")
	}
	fault.fail = false
	current, restored, err := svc.Get(ctx, j.ID)
	if err != nil || current.Status != domain.JobSucceeded || len(restored) != 1 || restored[0].OutputAssetID != "stable-generated-asset" || restored[0].Status != domain.SRSucceeded || assets.calls != 2 {
		t.Fatal("reconciliation did not restore persisted output")
	}
	if _, _, err := svc.Get(ctx, j.ID); err != nil || assets.calls != 2 {
		t.Fatal("terminal read repeated material save")
	}

	cancelled, cancelStep := seed("cancel")
	cancelAssets := &projectionAssets{after: func() {
		if err := jobs.UpdateStatus(ctx, cancelled.ID, domain.JobCancelled, ""); err != nil {
			t.Fatal("cancel setup failed")
		}
	}}
	if _, _, err := makeService(steps, cancelAssets).Get(ctx, cancelled.ID); err != nil {
		t.Fatal("late cancellation projection failed")
	}
	current, err = jobs.Get(ctx, cancelled.ID)
	persisted, stepErr := steps.Get(ctx, cancelStep.ID)
	if err != nil || stepErr != nil || current.Status != domain.JobCancelled || persisted.Status != domain.SRClaimed || persisted.OutputAssetID != "" {
		t.Fatal("late finalizer revived cancelled aggregate")
	}

	for _, boundary := range []string{"before_registration", "during_registration"} {
		regenerating, regenerateStep := seed("regenerate_" + boundary)
		candidates := []domain.Candidate{{Content: "old result"}}
		if steps.UpdateCandidates(ctx, regenerateStep.ID, candidates) != nil || steps.UpdateStatus(ctx, regenerateStep.ID, domain.SRNeedConfirm, nil) != nil {
			t.Fatal("regeneration seed failed")
		}
		regenerate := func() {
			frozen, _ := json.Marshal(candidates)
			decision := domain.ReviewDecision{ID: "review_" + boundary, JobID: regenerating.ID, StepRunID: regenerateStep.ID,
				ConductorWorkflowID: regenerating.ConductorWorkflowID, ConductorTaskID: "old-approval", TaskReferenceName: taskref.Reference("image", "approval") + "__1",
				Generation: 1, Decision: "regenerate", DecidedBy: regenerating.SubmittedBy, IdempotencyKey: "regenerate", CandidatesSnapshot: frozen,
				OutputJSON: json.RawMessage(`{"decision":"regenerate"}`)}
			decision.RequestDigest, _ = decision.Digest()
			if err := dal.NewTxRunner(db).RunInTx(ctx, func(txCtx context.Context) error {
				_, _, err := jobs.AcceptReview(txCtx, decision)
				return err
			}); err != nil {
				t.Fatal("regeneration review persistence failed")
			}
		}
		assetPort := &projectionAssets{}
		if boundary == "before_registration" {
			regenerate()
		} else {
			assetPort.after = regenerate
		}
		oldEngine := projectionEngine{execution: &domain.EngineExecution{Status: "RUNNING", Tasks: []domain.EngineTask{
			{TaskID: "old-success", ReferenceTaskName: taskref.Reference("image", "invoke") + "__1", Status: "COMPLETED"},
			{TaskID: "old-finalize", ReferenceTaskName: "step__image__finalize", Status: "COMPLETED", Output: map[string]any{"output_resource": map[string]any{"content": "old result"}}},
		}}}
		regenerateSvc := service.NewService(jobs, steps, dal.NewTxRunner(db), nil, nil, assetPort, nil, nil, oldEngine)
		regenerateSvc.ConfigureReviews(jobs)
		current, _, err = regenerateSvc.Get(ctx, regenerating.ID)
		persisted, stepErr = steps.Get(ctx, regenerateStep.ID)
		wantCalls := 0
		if boundary == "during_registration" {
			wantCalls = 1
		}
		if err != nil || stepErr != nil || current.Status != domain.JobRunning || current.RetryCount != 0 || persisted.Status != domain.SRClaimed || persisted.OutputAssetID != "" || assetPort.calls != wantCalls {
			t.Fatal("persisted regeneration boundary allowed old result reattachment")
		}
	}

	invoking, invokeStep := seed("invoke_rollback")
	if err := steps.UpdateCandidates(ctx, invokeStep.ID, []domain.Candidate{{Content: "preserved"}}); err != nil {
		t.Fatal("candidate seed failed")
	}
	invokeFault := &failProjectionStatus{StepRunRepo: steps, fail: true}
	invokeEngine := projectionEngine{execution: &domain.EngineExecution{Status: "RUNNING", Tasks: []domain.EngineTask{{ReferenceTaskName: "step__image__invoke", Status: "IN_PROGRESS"}}}}
	invokeSvc := service.NewService(jobs, invokeFault, dal.NewTxRunner(db), nil, nil, &projectionAssets{}, nil, nil, invokeEngine)
	invokeSvc.ConfigureReviews(jobs)
	if _, _, err := invokeSvc.Get(ctx, invoking.ID); err == nil {
		t.Fatal("invoke write failure ignored")
	}
	persisted, err = steps.Get(ctx, invokeStep.ID)
	if err != nil || len(persisted.Candidates) != 1 || persisted.Candidates[0].Content != "preserved" {
		t.Fatal("invoke candidate reset escaped transaction rollback")
	}

	for _, phase := range []string{"render", "invoke", "finalize", "approval"} {
		late, lateStep := seed("late_" + phase)
		taskStatus := "FAILED"
		if phase == "approval" {
			taskStatus = "IN_PROGRESS"
		}
		lateEngine := projectionEngine{execution: &domain.EngineExecution{Status: "FAILED", Tasks: []domain.EngineTask{{
			TaskID: "late-task", ReferenceTaskName: "step__image__" + phase, Status: taskStatus,
			Output: map[string]any{"prompt": "late prompt", "candidates": []domain.Candidate{{Content: "late candidate"}}},
			Input:  map[string]any{"candidates": []domain.Candidate{{Content: "late candidate"}}},
		}}}, afterRead: func() {
			if err := jobs.UpdateStatus(ctx, late.ID, domain.JobCancelled, ""); err != nil {
				t.Fatal("late cancel failed")
			}
		}}
		lateSvc := service.NewService(jobs, steps, dal.NewTxRunner(db), nil, nil, &projectionAssets{}, nil, nil, lateEngine)
		lateSvc.ConfigureReviews(jobs)
		if _, _, err := lateSvc.Get(ctx, late.ID); err != nil {
			t.Fatal("cancelled snapshot failed")
		}
		current, err = jobs.Get(ctx, late.ID)
		persisted, stepErr = steps.Get(ctx, lateStep.ID)
		if err != nil || stepErr != nil || current.Status != domain.JobCancelled || persisted.Status != domain.SRClaimed || persisted.RenderedPrompt != "" || len(persisted.Candidates) != 0 {
			t.Fatal("delayed phase projection modified cancelled aggregate")
		}
	}
	for _, phase := range []string{"render", "invoke", "finalize", "approval", "status"} {
		retrying, retryStep := seed("retry_snapshot_" + phase)
		assetPort := &projectionAssets{}
		task := domain.EngineTask{TaskID: "old-task", ReferenceTaskName: "step__image__" + phase, Status: "COMPLETED",
			Output: map[string]any{"prompt": "old prompt", "candidates": []domain.Candidate{{Content: "old candidate"}}, "output_resource": map[string]any{"content": "old result"}},
			Input:  map[string]any{"candidates": []domain.Candidate{{Content: "old candidate"}}},
		}
		if phase == "approval" {
			task.Status = "IN_PROGRESS"
		}
		snapshot := &domain.EngineExecution{Status: "FAILED", Tasks: []domain.EngineTask{task}}
		if phase == "status" {
			snapshot.Tasks = nil
		}
		oldEngine := projectionEngine{execution: snapshot, afterRead: func() {
			if err := jobs.AppendRetryHistory(ctx, retrying.ID, domain.RetryRecord{RetryAt: time.Now(), Reason: "isolated retry boundary"}, 1); err != nil {
				t.Fatal("retry transition failed")
			}
		}}
		retrySvc := service.NewService(jobs, steps, dal.NewTxRunner(db), nil, nil, assetPort, nil, nil, oldEngine)
		retrySvc.ConfigureReviews(jobs)
		current, _, err = retrySvc.Get(ctx, retrying.ID)
		persisted, stepErr = steps.Get(ctx, retryStep.ID)
		if err != nil || stepErr != nil || current.Status != domain.JobRunning || current.RetryCount != 1 || persisted.Status != domain.SRClaimed || persisted.RenderedPrompt != "" || len(persisted.Candidates) != 0 || persisted.OutputAssetID != "" || assetPort.calls != 0 {
			t.Fatal("stale snapshot overwrote persisted retry boundary")
		}
	}
	for _, phase := range []string{"render", "invoke", "approval", "finalize", "status"} {
		for _, decision := range []string{"regenerate", "accept"} {
			j, sr := seed("review_fence_" + phase + "_" + decision)
			candidates := []domain.Candidate{{Content: "preserved candidate"}}
			if steps.UpdateCandidates(ctx, sr.ID, candidates) != nil || steps.UpdateResolvedInputs(ctx, sr.ID, nil, "preserved prompt") != nil || steps.UpdateStatus(ctx, sr.ID, domain.SRNeedConfirm, nil) != nil {
				t.Fatal("review fence seed failed")
			}
			task := domain.EngineTask{TaskID: "old-task", ReferenceTaskName: taskref.Reference("image", phase) + "__1", Status: "COMPLETED",
				Input:  map[string]any{"candidates": []domain.Candidate{{Content: "late candidate"}}},
				Output: map[string]any{"prompt": "late prompt", "candidates": []domain.Candidate{{Content: "late candidate"}}}}
			if phase == "approval" {
				task.Status = "IN_PROGRESS"
			}
			if phase == "finalize" {
				task.ReferenceTaskName = "step__image__finalize"
				task.Status = "FAILED"
			}
			if phase == "status" {
				task.ReferenceTaskName = taskref.Reference("image", "loop")
				task.Output = map[string]any{"iteration": float64(1)}
			}
			engine := projectionEngine{execution: &domain.EngineExecution{Status: "FAILED", Tasks: []domain.EngineTask{task}}, afterRead: func() {
				frozen, _ := json.Marshal(candidates)
				output := map[string]any{"decision": decision}
				review := domain.ReviewDecision{ID: "review_" + phase + "_" + decision, JobID: j.ID, StepRunID: sr.ID, ConductorWorkflowID: j.ConductorWorkflowID,
					ConductorTaskID: "accepted-approval", TaskReferenceName: taskref.Reference("image", "approval") + "__1", Generation: 1, Decision: decision, DecidedBy: j.SubmittedBy,
					IdempotencyKey: "boundary", CandidatesSnapshot: frozen}
				if decision == "accept" {
					index := 0
					review.SelectedIndex = &index
					output["selected_index"] = 0
					output["selected_candidate"] = candidates[0]
				}
				review.OutputJSON, _ = json.Marshal(output)
				review.RequestDigest, _ = review.Digest()
				if err := dal.NewTxRunner(db).RunInTx(ctx, func(tx context.Context) error { _, _, err := jobs.AcceptReview(tx, review); return err }); err != nil {
					t.Fatal("review fence acceptance failed")
				}
			}}
			assets := &projectionAssets{}
			svc := service.NewService(jobs, steps, dal.NewTxRunner(db), nil, nil, assets, nil, nil, engine)
			svc.ConfigureReviews(jobs)
			current, _, err := svc.Get(ctx, j.ID)
			persisted, stepErr := steps.Get(ctx, sr.ID)
			if err != nil || stepErr != nil || current.Status != domain.JobRunning || current.RetryCount != 0 || persisted.Status != domain.SRClaimed || persisted.RenderedPrompt != "preserved prompt" || len(persisted.Candidates) != 1 || persisted.Candidates[0].Content != "preserved candidate" || persisted.OutputAssetID != "" || assets.calls != 0 {
				t.Fatal("persisted review did not fence late phase/job snapshot")
			}
		}
	}
}
