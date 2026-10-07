package job

import (
	"context"
	"errors"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/shared/taskref"
)

func TestCurrentGenerationResultIdentity(t *testing.T) {
	step := &domain.StepRun{ID: "stable-step", StepKey: "image"}
	invoke := func(id, suffix, status string) domain.EngineTask {
		return domain.EngineTask{TaskID: id, ReferenceTaskName: taskref.Reference("image", "invoke") + suffix, Status: status}
	}
	execution := &domain.EngineExecution{Tasks: []domain.EngineTask{
		invoke("old-success", "__1", "COMPLETED"), invoke("failed-attempt", "__2", "FAILED"), invoke("new-success", "__2", "COMPLETED"),
		{TaskID: "finalizer-a", ReferenceTaskName: "step__image__finalize", Status: "COMPLETED"},
	}}
	key, err := currentGenerationResultKey("engine", step, execution)
	want, _ := domain.GenerationResultKey("engine", step.ID, "new-success")
	if err != nil || key != want {
		t.Fatal("wrong successful capability identity selected", err)
	}
	execution.Tasks[3].TaskID = "finalizer-retry"
	replay, err := currentGenerationResultKey("engine", step, execution)
	if err != nil || replay != key {
		t.Fatal("finalize retry changed generation identity")
	}
	execution.Tasks[2].TaskID = "successful-retry"
	next, err := currentGenerationResultKey("engine", step, execution)
	if err != nil || next == key {
		t.Fatal("new successful task reused prior generation identity")
	}
	for _, tc := range []struct {
		name  string
		tasks []domain.EngineTask
	}{
		{"missing", nil},
		{"failed only", []domain.EngineTask{invoke("failed", "__1", "FAILED")}},
		{"missing identity", []domain.EngineTask{invoke("", "__1", "COMPLETED")}},
		{"ambiguous", []domain.EngineTask{invoke("a", "__1", "COMPLETED"), invoke("b", "__1", "COMPLETED")}},
		{"pending newer round", []domain.EngineTask{invoke("old", "__1", "COMPLETED"), invoke("next", "__2", "IN_PROGRESS")}},
		{"pending same round", []domain.EngineTask{invoke("old", "__1", "COMPLETED"), invoke("next", "__1", "SCHEDULED")}},
		{"loop advanced without invoke", []domain.EngineTask{invoke("old", "__1", "COMPLETED"), {ReferenceTaskName: taskref.Reference("image", "loop"), Output: map[string]any{"iteration": float64(2)}}}},
		{"other step", []domain.EngineTask{{TaskID: "a", ReferenceTaskName: taskref.Reference("other", "invoke") + "__1", Status: "COMPLETED"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := currentGenerationResultKey("engine", step, &domain.EngineExecution{Tasks: tc.tasks}); err == nil {
				t.Fatal("unsafe identity accepted")
			}
		})
	}
}

type generationAssetSaver struct {
	keys []string
}

func (s *generationAssetSaver) UpsertGeneratedAsset(_ context.Context, req *client.UpsertGenAssetReq) (string, error) {
	s.keys = append(s.keys, req.ResultKey)
	return "gen_" + req.ResultKey, nil
}

func TestNativeFinalizeQueryPreservesExistingAssetWithoutRegistration(t *testing.T) {
	svc, _, _, engine, sr := projectionFixture()
	assets := &generationAssetSaver{}
	svc.materials = assets
	if _, _, err := svc.Get(context.Background(), sr.JobID); err != nil {
		t.Fatal(err)
	}
	first := sr.OutputAssetID
	engine.execution.Tasks[0].Output = map[string]any{"output_resource_id": "gen_late", "output_resource": map[string]any{"id": "gen_late"}}
	if _, _, err := svc.Get(context.Background(), sr.JobID); err != nil {
		t.Fatal(err)
	}
	if len(assets.keys) != 0 || first != "gen_generated" || sr.OutputAssetID != first {
		t.Fatal("query re-registered or replaced a successful branch")
	}
}

func TestNativeQueryProjectsRegisteredOutputWithoutRecomputingProviderExecution(t *testing.T) {
	svc, _, _, engine, sr := projectionFixture()
	assets := &generationAssetSaver{}
	svc.materials = assets
	engine.execution.Tasks = append(engine.execution.Tasks, domain.EngineTask{TaskID: "another-success", ReferenceTaskName: "step__s1__invoke", Status: "COMPLETED"})
	if _, _, err := svc.Get(context.Background(), sr.JobID); err != nil || len(assets.keys) != 0 || sr.OutputAssetID != "gen_generated" {
		t.Fatal("query reran resource registration or provider validation", err)
	}
}

func TestFinalizeProjectionRetryKeepsRegisteredAssetIdentity(t *testing.T) {
	svc, _, steps, _, sr := projectionFixture()
	assets := &generationAssetSaver{}
	svc.materials = assets
	repo := &projectionStepRepo{mockStepRunRepo: steps, outputErr: errors.New("injected local failure")}
	svc.stepRunRepo = repo
	if _, _, err := svc.Get(context.Background(), sr.JobID); err == nil {
		t.Fatal("projection error ignored")
	}
	repo.outputErr = nil
	if _, _, err := svc.Get(context.Background(), sr.JobID); err != nil {
		t.Fatal(err)
	}
	if len(assets.keys) != 0 || sr.OutputAssetID != "gen_generated" {
		t.Fatal("projection registered another asset")
	}
}

type generationReviewRepo struct {
	domain.ReviewRepo
	reviews []domain.ReviewDecision
}

func (r *generationReviewRepo) LatestReviewsByJob(context.Context, string) ([]domain.ReviewDecision, error) {
	return r.reviews, nil
}
func (*generationReviewRepo) LockReviewProjection(context.Context, string, string) error { return nil }

func TestRegenerationReviewFencesLateFinalizeProjection(t *testing.T) {
	for _, timing := range []string{"before_query", "after_snapshot"} {
		t.Run(timing, func(t *testing.T) {
			svc, jobs, _, engine, sr := projectionFixture()
			engine.execution.Status = "RUNNING"
			engine.execution.Tasks[1].ReferenceTaskName = taskref.Reference(sr.StepKey, "invoke") + "__1"
			reviews := &generationReviewRepo{}
			svc.reviews, svc.tx = reviews, commandReviewTx{}
			regenerate := func() {
				reviews.reviews = []domain.ReviewDecision{{JobID: sr.JobID, StepRunID: sr.ID, ConductorWorkflowID: "execution", Generation: 1, Decision: "regenerate", ConductorTaskID: "old"}}
			}
			if timing == "before_query" {
				regenerate()
			} else {
				svc.engine = delayedProjectionEngine{WorkflowEngine: engine, afterRead: regenerate}
			}
			assets := &projectionAssetSaver{id: "unexpected"}
			svc.materials = assets
			current, _, err := svc.Get(context.Background(), sr.JobID)
			if err != nil || current.Status != domain.JobRunning || sr.Status == domain.SRSucceeded || sr.OutputAssetID != "" || jobs.jobs[sr.JobID].RetryCount != 0 || assets.calls != 0 {
				t.Fatal("late projection changed an accepted regeneration", err)
			}
		})
	}
}

func TestGenerationSnapshotRequiresMatchingApprovalTask(t *testing.T) {
	svc, _, _, _, sr := projectionFixture()
	reviews := &generationReviewRepo{reviews: []domain.ReviewDecision{{StepRunID: sr.ID, ConductorWorkflowID: "execution", Generation: 2, Decision: "accept", ConductorTaskID: "approved-task"}}}
	svc.reviews = reviews
	execution := &domain.EngineExecution{Tasks: []domain.EngineTask{
		{TaskID: "successful", ReferenceTaskName: taskref.Reference(sr.StepKey, "invoke") + "__2", Status: "COMPLETED"},
		{TaskID: "wrong-task", ReferenceTaskName: taskref.Reference(sr.StepKey, "approval") + "__2", Status: "COMPLETED"},
	}}
	ctx := withGenerationSnapshot(context.Background(), "execution", sr, execution)
	if !errors.Is(svc.checkGenerationSnapshot(ctx, sr.JobID, sr.ID), errExecutionSnapshotStale) {
		t.Fatal("different approval task accepted")
	}
	execution.Tasks[1].TaskID = "approved-task"
	ctx = withGenerationSnapshot(context.Background(), "execution", sr, execution)
	if err := svc.checkGenerationSnapshot(ctx, sr.JobID, sr.ID); err != nil {
		t.Fatal("matching approval task rejected", err)
	}
}

func TestReviewSupersedesLatePhaseAndJobStatusSnapshots(t *testing.T) {
	for _, phase := range []string{"render", "invoke", "approval", "finalize", "status"} {
		for _, decision := range []string{"regenerate", "accept"} {
			t.Run(phase+"/"+decision, func(t *testing.T) {
				svc, jobs, _, engine, sr := projectionFixture()
				sr.RenderedPrompt = "current prompt"
				sr.Candidates = []domain.Candidate{{Content: "current candidate"}}
				engine.execution.Status = "FAILED"
				task := domain.EngineTask{TaskID: "old-task", ReferenceTaskName: taskref.Reference(sr.StepKey, phase) + "__1", Status: "COMPLETED",
					Output: map[string]any{"prompt": "late old prompt", "candidates": []domain.Candidate{{Content: "late old candidate"}}},
					Input:  map[string]any{"candidates": []domain.Candidate{{Content: "late old candidate"}}}}
				if phase == "approval" {
					task.Status = "IN_PROGRESS"
				}
				if phase == "finalize" {
					task.ReferenceTaskName = "step__s1__finalize"
					task.Status = "FAILED"
				}
				if phase == "status" {
					task.ReferenceTaskName = taskref.Reference(sr.StepKey, "loop")
					task.Output = map[string]any{"iteration": float64(1)}
				}
				engine.execution.Tasks = []domain.EngineTask{task}
				reviews := &generationReviewRepo{}
				svc.reviews, svc.tx = reviews, commandReviewTx{}
				svc.engine = delayedProjectionEngine{WorkflowEngine: engine, afterRead: func() {
					reviews.reviews = []domain.ReviewDecision{{JobID: sr.JobID, StepRunID: sr.ID, ConductorWorkflowID: "execution", Generation: 1, Decision: decision, ConductorTaskID: "accepted-approval"}}
				}}
				current, _, err := svc.Get(context.Background(), sr.JobID)
				if err != nil || current.Status != domain.JobRunning || jobs.jobs[sr.JobID].RetryCount != 0 || sr.Status != domain.SRClaimed || sr.RenderedPrompt != "current prompt" || len(sr.Candidates) != 1 || sr.Candidates[0].Content != "current candidate" || sr.OutputAssetID != "" {
					t.Fatal("older snapshot changed accepted review state", err)
				}
			})
		}
	}
}

func TestNewGenerationProjectionNotBlockedByPreviousRegeneration(t *testing.T) {
	svc, _, _, engine, sr := projectionFixture()
	reviews := &generationReviewRepo{reviews: []domain.ReviewDecision{{JobID: sr.JobID, StepRunID: sr.ID, ConductorWorkflowID: "execution", Generation: 1, Decision: "regenerate"}}}
	svc.reviews, svc.tx = reviews, commandReviewTx{}
	engine.execution.Status = "RUNNING"
	engine.execution.Tasks = []domain.EngineTask{{TaskID: "new-render", ReferenceTaskName: taskref.Reference(sr.StepKey, "render") + "__2", Status: "COMPLETED", Output: map[string]any{"prompt": "new round prompt"}}}
	if _, _, err := svc.Get(context.Background(), sr.JobID); err != nil || sr.RenderedPrompt != "new round prompt" {
		t.Fatal("new round blocked by old review", err)
	}
}

func TestRetryCycleReusesNumericGenerationWithoutReusingReview(t *testing.T) {
	svc, jobs, _, engine, sr := projectionFixture()
	oldEpoch := 0
	reviews := &generationReviewRepo{reviews: []domain.ReviewDecision{{JobID: sr.JobID, StepRunID: sr.ID, ConductorWorkflowID: "execution", Generation: 2, Decision: "regenerate", AcceptedRetryCount: &oldEpoch}}}
	svc.reviews, svc.tx = reviews, commandReviewTx{}
	jobs.jobs[sr.JobID].RetryCount = 1
	engine.execution.Status = "RUNNING"
	engine.execution.Tasks = []domain.EngineTask{{TaskID: "new-cycle-render", ReferenceTaskName: taskref.Reference(sr.StepKey, "render") + "__2", Status: "COMPLETED", Output: map[string]any{"prompt": "new cycle prompt"}}}
	if _, _, err := svc.Get(context.Background(), sr.JobID); err != nil || sr.RenderedPrompt != "new cycle prompt" {
		t.Fatal("prior cycle review blocked same numeric generation", err)
	}
	currentEpoch := 1
	reviews.reviews[0].AcceptedRetryCount = &currentEpoch
	engine.execution.Tasks[0].Output = map[string]any{"prompt": "late after new decision"}
	if _, _, err := svc.Get(context.Background(), sr.JobID); err != nil || sr.RenderedPrompt != "new cycle prompt" {
		t.Fatal("same cycle review failed to fence old projection", err)
	}
}
