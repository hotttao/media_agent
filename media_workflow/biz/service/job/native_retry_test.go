package job

import (
	"context"
	"errors"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
)

func TestStepRetryUsesParentEngineStateAndPreservesSuccessfulBranch(t *testing.T) {
	svc, jobs, steps := newTestService(false)
	j := &domain.Job{ID: "job1", Status: domain.JobRunning, ConductorWorkflowID: "engine", RetryCount: 100}
	jobs.jobs[j.ID] = j // Deliberately stale, and beyond the old local retry limit.
	failed := makeClaimedStepRun(steps, false)
	failed.Status = domain.SRClaimed
	failed.RenderedPrompt = "already rendered"
	success := &domain.StepRun{ID: "branch", JobID: j.ID, StepKey: "branch", Status: domain.SRSucceeded, OutputAssetID: "retained-asset"}
	steps.items[success.ID] = success
	jobs.stepRuns[j.ID] = []*domain.StepRun{failed, success}
	engine := &mockWorkflowEngine{execution: &domain.EngineExecution{WorkflowInstanceID: "engine", Status: "FAILED"}}
	svc.engine = engine
	if _, err := svc.RetryStepRun(context.Background(), failed.ID, "continue", nil); err != nil {
		t.Fatal(err)
	}
	if engine.retryCalls != 1 || engine.rerunTaskID != "" || engine.rerunInput != nil || success.Status != domain.SRSucceeded ||
		success.OutputAssetID != "retained-asset" || failed.RenderedPrompt != "already rendered" || j.RetryCount != 100 {
		t.Fatal("retry imposed local recovery scope, limits or state resets")
	}
	if _, err := svc.RetryStepRun(context.Background(), failed.ID, "repeat", nil); err != nil || engine.retryCalls != 1 || len(j.RetryHistory) != 1 {
		t.Fatal("running execution was retried again", err)
	}
}

func TestQueryRefreshesFailedCacheAndIgnoresSupersededNativeAttempt(t *testing.T) {
	for _, endpoint := range []string{"job", "step", "list"} {
		t.Run(endpoint, func(t *testing.T) {
			svc, jobs, steps := newTestService(false)
			j := &domain.Job{ID: "job1", Status: domain.JobFailed, ConductorWorkflowID: "engine"}
			jobs.jobs[j.ID] = j
			sr := makeClaimedStepRun(steps, false)
			sr.Status = domain.SRFailed
			jobs.stepRuns[j.ID] = []*domain.StepRun{sr}
			svc.engine = &mockWorkflowEngine{execution: &domain.EngineExecution{WorkflowInstanceID: "engine", Status: "RUNNING", Tasks: []domain.EngineTask{
				{TaskID: "new", ReferenceTaskName: "step__s1__invoke", Status: "IN_PROGRESS", RetryCount: 1},
				// Reverse task order to prove the superseded failure cannot overwrite the current attempt.
				{TaskID: "old", ReferenceTaskName: "step__s1__invoke", Status: "FAILED", Retried: true},
			}}}
			var err error
			switch endpoint {
			case "job":
				_, _, err = svc.Get(context.Background(), j.ID)
			case "step":
				_, err = svc.GetStepRun(context.Background(), sr.ID)
			case "list":
				_, _, err = svc.List(context.Background(), domain.ListQuery{})
			}
			if err != nil || j.Status != domain.JobRunning || sr.Status != domain.SRClaimed || sr.RetryCount != 1 {
				t.Fatal("cache vetoed engine recovery or old attempt overwrote new task", err)
			}
		})
	}
}

func TestRetryDoesNotMutateStateOnUnconfirmedEngineFailure(t *testing.T) {
	svc, jobs, steps := newTestService(false)
	j := &domain.Job{ID: "job1", Status: domain.JobFailed, ConductorWorkflowID: "engine"}
	jobs.jobs[j.ID] = j
	sr := makeClaimedStepRun(steps, false)
	sr.Status = domain.SRFailed
	sr.Candidates = []domain.Candidate{{Content: "retained"}}
	engine := &mockWorkflowEngine{execution: &domain.EngineExecution{Status: "FAILED"}, retryErr: errors.New("private engine error")}
	svc.engine = engine
	if _, err := svc.Retry(context.Background(), j.ID, "continue", nil); err == nil || err.Error() == "private engine error" {
		t.Fatal("unconfirmed outcome was reported successful or leaked upstream details")
	}
	if engine.retryCalls != 1 || j.Status != domain.JobFailed || len(j.RetryHistory) != 0 || sr.Status != domain.SRFailed || len(sr.Candidates) != 1 {
		t.Fatal("failed delivery reset local state or issued another retry")
	}
}

func TestRetryRejectsOtherNativeOperations(t *testing.T) {
	for _, status := range []string{"PAUSED", "TERMINATED", "COMPLETED"} {
		t.Run(status, func(t *testing.T) {
			svc, jobs, _ := newTestService(false)
			jobs.jobs["job1"] = &domain.Job{ID: "job1", Status: domain.JobFailed, ConductorWorkflowID: "engine"}
			engine := &mockWorkflowEngine{execution: &domain.EngineExecution{Status: status}}
			svc.engine = engine
			if _, err := svc.Retry(context.Background(), "job1", "continue", nil); err == nil || engine.retryCalls != 0 {
				t.Fatal("retry silently became restart, resume or rerun")
			}
		})
	}
}
