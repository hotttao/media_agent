package job

import (
	"context"
	"errors"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
)

type projectionAssetSaver struct {
	id    string
	err   error
	after func()
	calls int
}

type delayedProjectionEngine struct {
	domain.WorkflowEngine
	afterRead func()
}

type projectionLockRepo struct {
	domain.ReviewRepo
	locks int
}

func (r *projectionLockRepo) LockReviewProjection(context.Context, string, string) error {
	r.locks++
	return nil
}

func (e delayedProjectionEngine) Get(ctx context.Context, id string) (*domain.EngineExecution, error) {
	result, err := e.WorkflowEngine.Get(ctx, id)
	e.afterRead()
	return result, err
}

func (s *projectionAssetSaver) UpsertGeneratedAsset(context.Context, *client.UpsertGenAssetReq) (string, error) {
	s.calls++
	if s.after != nil {
		s.after()
	}
	return s.id, s.err
}

type projectionStepRepo struct {
	*mockStepRunRepo
	outputErr, statusErr, candidatesErr, inputsErr error
}

func (r *projectionStepRepo) UpdateCandidates(ctx context.Context, id string, candidates []domain.Candidate) error {
	if r.candidatesErr != nil {
		return r.candidatesErr
	}
	return r.mockStepRunRepo.UpdateCandidates(ctx, id, candidates)
}

func (r *projectionStepRepo) UpdateResolvedInputs(ctx context.Context, id string, inputs map[string]any, prompt string) error {
	if r.inputsErr != nil {
		return r.inputsErr
	}
	return r.mockStepRunRepo.UpdateResolvedInputs(ctx, id, inputs, prompt)
}

func (r *projectionStepRepo) UpdateOutput(ctx context.Context, id, asset string) error {
	if r.outputErr != nil {
		return r.outputErr
	}
	return r.mockStepRunRepo.UpdateOutput(ctx, id, asset)
}

func (r *projectionStepRepo) UpdateStatus(ctx context.Context, id string, status domain.StepRunStatus, updates map[string]any) error {
	if r.statusErr != nil {
		return r.statusErr
	}
	return r.mockStepRunRepo.UpdateStatus(ctx, id, status, updates)
}

func projectionFixture() (*Service, *mockJobRepo, *mockStepRunRepo, *mockWorkflowEngine, *domain.StepRun) {
	svc, jobs, steps := newTestService(false)
	sr := makeClaimedStepRun(steps, false)
	jobs.jobs[sr.JobID] = &domain.Job{ID: sr.JobID, WorkflowID: "wf1", Status: domain.JobRunning, ConductorWorkflowID: "execution"}
	jobs.stepRuns[sr.JobID] = []*domain.StepRun{sr}
	engine := &mockWorkflowEngine{execution: &domain.EngineExecution{Status: "COMPLETED", Tasks: []domain.EngineTask{{
		ReferenceTaskName: "step__s1__finalize", Status: "COMPLETED", Output: map[string]any{"output_resource_id": "gen_generated", "output_resource": map[string]any{"id": "gen_generated", "content": "result"}},
	}, {TaskID: "successful-invoke", ReferenceTaskName: "step__s1__invoke", Status: "COMPLETED"}}}}
	svc.engine = engine
	return svc, jobs, steps, engine, sr
}

func TestFinalizePersistenceFailureNeverCompletesJob(t *testing.T) {
	failure := errors.New("injected persistence failure")
	for _, stage := range []string{"missing_identity", "empty_identity", "output", "status"} {
		t.Run(stage, func(t *testing.T) {
			svc, jobs, steps, engine, sr := projectionFixture()
			assets := &projectionAssetSaver{id: "generated"}
			repo := &projectionStepRepo{mockStepRunRepo: steps}
			switch stage {
			case "missing_identity":
				delete(engine.execution.Tasks[0].Output, "output_resource_id")
			case "empty_identity":
				engine.execution.Tasks[0].Output["output_resource_id"] = " "
			case "output":
				repo.outputErr = failure
			case "status":
				repo.statusErr = failure
			}
			svc.materials, svc.stepRunRepo = assets, repo
			if err := svc.reconcileConductor(context.Background(), jobs.jobs[sr.JobID]); err == nil {
				t.Fatal("failed persistence was treated as completion")
			}
			if jobs.jobs[sr.JobID].Status != domain.JobRunning || sr.Status == domain.SRSucceeded {
				t.Fatal("failed persistence advanced local success")
			}
			// Restore a registered engine output or the local projection write;
			// queries never register an asset themselves.
			engine.execution.Tasks[0].Output["output_resource_id"] = "gen_generated"
			repo.outputErr, repo.statusErr = nil, nil
			if err := svc.reconcileConductor(context.Background(), jobs.jobs[sr.JobID]); err != nil {
				t.Fatal(err)
			}
			if jobs.jobs[sr.JobID].Status != domain.JobSucceeded || sr.OutputAssetID != "gen_generated" || assets.calls != 0 {
				t.Fatal("persistence recovery did not complete")
			}
		})
	}
}

func TestConductorCompletionRequiresAllPersistedOutputs(t *testing.T) {
	for _, state := range []string{"no_steps", "missing_finalize", "succeeded_without_output"} {
		t.Run(state, func(t *testing.T) {
			svc, jobs, steps, engine, sr := projectionFixture()
			engine.execution.Tasks = nil
			if state == "no_steps" {
				delete(steps.items, sr.ID)
			}
			if state == "succeeded_without_output" {
				sr.Status = domain.SRSucceeded
			}
			if err := svc.reconcileConductor(context.Background(), jobs.jobs[sr.JobID]); err == nil {
				t.Fatal("engine completion alone incorrectly proved business completion")
			}
			if jobs.jobs[sr.JobID].Status != domain.JobRunning {
				t.Fatal("job advanced")
			}
		})
	}
}

func TestNativeFinalizeDoesNotCompleteRunningEngine(t *testing.T) {
	svc, jobs, _, engine, sr := projectionFixture()
	engine.execution.Status = "RUNNING"
	if err := svc.reconcileConductor(context.Background(), jobs.jobs[sr.JobID]); err != nil {
		t.Fatal(err)
	}
	if sr.Status != domain.SRSucceeded || jobs.jobs[sr.JobID].Status != domain.JobRunning {
		t.Fatal("local DAG completion overrode running engine")
	}
}

func TestLateFinalizeSnapshotDoesNotReviveCancellation(t *testing.T) {
	svc, jobs, _, engine, sr := projectionFixture()
	svc.engine = delayedProjectionEngine{WorkflowEngine: engine, afterRead: func() { jobs.jobs[sr.JobID].Status = domain.JobCancelled }}
	if err := svc.reconcileConductor(context.Background(), jobs.jobs[sr.JobID]); err != nil {
		t.Fatal(err)
	}
	if jobs.jobs[sr.JobID].Status != domain.JobCancelled || sr.Status == domain.SRSucceeded || sr.OutputAssetID != "" {
		t.Fatal("late projection revived cancellation")
	}
}

func TestExecutionProjectionRejectsUnpersistedOrMalformedCandidates(t *testing.T) {
	for _, stage := range []string{"render", "invoke_candidates", "invoke_status", "malformed_candidates", "approval_candidates", "finalize_status"} {
		t.Run(stage, func(t *testing.T) {
			svc, jobs, steps, engine, sr := projectionFixture()
			failure := errors.New("injected write failure")
			repo := &projectionStepRepo{mockStepRunRepo: steps}
			svc.stepRunRepo = repo
			task := domain.EngineTask{ReferenceTaskName: "step__s1__invoke", Status: "COMPLETED", Output: map[string]any{"candidates": []domain.Candidate{{Content: "result"}}}}
			switch stage {
			case "render":
				task.ReferenceTaskName, task.Output = "step__s1__render", map[string]any{"prompt": "prompt"}
				repo.inputsErr = failure
			case "invoke_candidates":
				repo.candidatesErr = failure
			case "invoke_status":
				task.Status, task.Output, repo.statusErr = "IN_PROGRESS", nil, failure
			case "malformed_candidates":
				task.Output["candidates"] = "not an array"
			case "approval_candidates":
				task.ReferenceTaskName, task.Status = "step__s1__approval", "IN_PROGRESS"
				task.Input, task.Output = task.Output, nil
				repo.candidatesErr = failure
			case "finalize_status":
				task.ReferenceTaskName, task.Status, task.Output = "step__s1__finalize", "FAILED", nil
				repo.statusErr = failure
			}
			engine.execution.Tasks = []domain.EngineTask{task}
			if err := svc.reconcileConductor(context.Background(), jobs.jobs[sr.JobID]); err == nil {
				t.Fatal("invalid or unpersisted task projection silently accepted")
			}
			if jobs.jobs[sr.JobID].Status != domain.JobRunning {
				t.Fatal("failed projection completed job")
			}
		})
	}
}

func TestDelayedEngineProjectionDoesNotOverwriteCancelledJob(t *testing.T) {
	for _, phase := range []string{"render", "invoke", "finalize"} {
		t.Run(phase, func(t *testing.T) {
			svc, jobs, _, engine, sr := projectionFixture()
			engine.execution.Status = "FAILED"
			engine.execution.Tasks = []domain.EngineTask{{ReferenceTaskName: "step__s1__" + phase, Status: "FAILED", Output: map[string]any{"prompt": "late prompt", "candidates": []domain.Candidate{{Content: "late candidate"}}}}}
			snapshot := *jobs.jobs[sr.JobID]
			svc.engine = delayedProjectionEngine{WorkflowEngine: engine, afterRead: func() { jobs.jobs[sr.JobID].Status = domain.JobCancelled }}
			if err := svc.reconcileConductor(context.Background(), &snapshot); err != nil {
				t.Fatal(err)
			}
			if jobs.jobs[sr.JobID].Status != domain.JobCancelled || sr.Status != domain.SRClaimed || len(sr.Candidates) != 0 || sr.RenderedPrompt != "" {
				t.Fatal("stale engine snapshot changed cancelled aggregate")
			}
		})
	}
}

func TestDelayedInvokeCannotEraseFinalizedParallelBranch(t *testing.T) {
	svc, jobs, _, engine, sr := projectionFixture()
	sr.Status, sr.OutputAssetID = domain.SRSucceeded, "persisted"
	sr.Candidates = []domain.Candidate{{Content: "persisted candidate"}}
	engine.execution.Status = "RUNNING"
	engine.execution.Tasks = []domain.EngineTask{{ReferenceTaskName: "step__s1__invoke", Status: "IN_PROGRESS"}}
	if err := svc.reconcileConductor(context.Background(), jobs.jobs[sr.JobID]); err != nil {
		t.Fatal(err)
	}
	if sr.Status != domain.SRSucceeded || sr.OutputAssetID != "persisted" || len(sr.Candidates) != 1 {
		t.Fatal("late invocation regressed finalized branch")
	}
}

func TestWaitingApprovalCannotReopenFinalizedParallelBranch(t *testing.T) {
	svc, _, _, _, sr := projectionFixture()
	sr.Status, sr.OutputAssetID = domain.SRSucceeded, "persisted"
	locks := &projectionLockRepo{}
	svc.tx, svc.reviews = commandReviewTx{}, locks
	if err := svc.projectWaitingReview(context.Background(), sr.JobID, sr.ID, domain.EngineTask{
		TaskID: "old-approval", Status: "IN_PROGRESS", Input: map[string]any{"candidates": []domain.Candidate{{Content: "old"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if locks.locks != 1 || sr.Status != domain.SRSucceeded || len(sr.Candidates) != 0 {
		t.Fatal("late approval reopened finalized parallel branch")
	}
}

func TestRetrySupersedesInFlightExecutionSnapshot(t *testing.T) {
	for _, phase := range []string{"render", "invoke", "finalize", "status"} {
		for _, changed := range []string{"retry_count", "engine_id"} {
			t.Run(phase+"/"+changed, func(t *testing.T) {
				svc, jobs, _, engine, sr := projectionFixture()
				assets := &projectionAssetSaver{id: "generated"}
				svc.materials = assets
				engine.execution.Status = "FAILED"
				task := domain.EngineTask{ReferenceTaskName: "step__s1__" + phase, Status: "COMPLETED", Output: map[string]any{
					"prompt": "old prompt", "candidates": []domain.Candidate{{Content: "old candidate"}}, "output_resource": map[string]any{"content": "old result"},
				}}
				engine.execution.Tasks = []domain.EngineTask{task}
				if phase == "status" {
					engine.execution.Tasks = nil
				}
				svc.engine = delayedProjectionEngine{WorkflowEngine: engine, afterRead: func() {
					if changed == "retry_count" {
						jobs.jobs[sr.JobID].RetryCount++
					} else {
						jobs.jobs[sr.JobID].ConductorWorkflowID = "new-execution"
					}
				}}
				got, _, err := svc.Get(context.Background(), sr.JobID)
				if err != nil || got.Status != domain.JobRunning || sr.Status != domain.SRClaimed || sr.RenderedPrompt != "" || len(sr.Candidates) != 0 || sr.OutputAssetID != "" || assets.calls != 0 {
					t.Fatal("old snapshot changed restarted execution", err)
				}
			})
		}
	}
}

func TestQueryNeverTriggersAssetRegistration(t *testing.T) {
	svc, jobs, _, _, sr := projectionFixture()
	assets := &projectionAssetSaver{id: "unexpected", after: func() { jobs.jobs[sr.JobID].RetryCount++ }}
	svc.materials = assets
	current, _, err := svc.Get(context.Background(), sr.JobID)
	if err != nil || current.Status != domain.JobSucceeded || current.RetryCount != 0 || sr.OutputAssetID != "gen_generated" || assets.calls != 0 {
		t.Fatal("query performed registration instead of pure projection", err)
	}
}
