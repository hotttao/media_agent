package job

import (
	"context"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
)

func TestRegenerationDoesNotRerunLegacyOrNonwaitingExecution(t *testing.T) {
	for _, tc := range []struct{ name, ref, status, workflowStatus string }{
		{"legacy", "step__s1__approval", "IN_PROGRESS", "RUNNING"},
		{"already decided", "step_7331_approval__1", "COMPLETED", "RUNNING"},
		{"terminal", "step_7331_approval__1", "IN_PROGRESS", "COMPLETED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, jobs, steps := newTestService(false)
			engine := &mockWorkflowEngine{execution: &domain.EngineExecution{Status: tc.workflowStatus, Tasks: []domain.EngineTask{{ReferenceTaskName: tc.ref, Status: tc.status}}}}
			svc.engine = engine
			jobs.jobs["job1"] = &domain.Job{ID: "job1", Status: domain.JobNeedConfirm, ConductorWorkflowID: "execution"}
			sr := makeClaimedStepRun(steps, false)
			sr.Status = domain.SRNeedConfirm
			sr.Candidates = []domain.Candidate{{Content: "old"}}
			if _, err := svc.Regenerate(context.Background(), sr.ID); err == nil {
				t.Fatal("unsupported regeneration succeeded")
			}
			if engine.completedRef != "" || engine.rerunTaskID != "" || engine.terminated || sr.Status != domain.SRNeedConfirm || len(sr.Candidates) != 1 {
				t.Fatal("rejected regeneration changed execution")
			}
		})
	}
}

func TestConfirmRejectsStaleLocalCandidates(t *testing.T) {
	svc, jobs, steps := newTestService(false)
	engine := &mockWorkflowEngine{execution: &domain.EngineExecution{Status: "RUNNING", Tasks: []domain.EngineTask{{ReferenceTaskName: "step_7331_approval__2", Status: "IN_PROGRESS", Input: map[string]any{"candidates": []domain.Candidate{{Content: "new"}}}}}}}
	svc.engine = engine
	jobs.jobs["job1"] = &domain.Job{ID: "job1", Status: domain.JobNeedConfirm, ConductorWorkflowID: "execution"}
	sr := makeClaimedStepRun(steps, false)
	sr.Status = domain.SRNeedConfirm
	sr.Candidates = []domain.Candidate{{Content: "old"}}
	if _, err := svc.Confirm(context.Background(), sr.ID, 0); err == nil || engine.completedRef != "" {
		t.Fatal("stale local candidate completed a newer approval")
	}
	sr.Candidates = []domain.Candidate{{Content: "new"}}
	if _, err := svc.Confirm(context.Background(), sr.ID, 0); err != nil || engine.completedRef != "step_7331_approval__2" || engine.completedOutput["decision"] != "accept" {
		t.Fatalf("current candidate was not accepted: %v", err)
	}
}

func TestReconcileIgnoresOldGenerationRegardlessOfTaskOrder(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		svc, jobs, steps := newTestService(false)
		sr := makeClaimedStepRun(steps, false)
		sr.Candidates = []domain.Candidate{{Content: "stale"}}
		job := &domain.Job{ID: "job1", Status: domain.JobRunning, ConductorWorkflowID: "execution"}
		jobs.jobs[job.ID] = job
		jobs.stepRuns[job.ID] = []*domain.StepRun{sr}
		tasks := []domain.EngineTask{
			{ReferenceTaskName: "step_7331_invoke__2", Status: "COMPLETED", Output: map[string]any{"candidates": []domain.Candidate{{Content: "new"}}}},
			{ReferenceTaskName: "step_7331_approval__2", Status: "IN_PROGRESS", Input: map[string]any{"candidates": []domain.Candidate{{Content: "new"}}}},
			{ReferenceTaskName: "step_7331_approval__1", Status: "COMPLETED", Input: map[string]any{"candidates": []domain.Candidate{{Content: "old"}}}},
			{ReferenceTaskName: "step_7331_invoke__1", Status: "FAILED", Output: map[string]any{"candidates": []domain.Candidate{{Content: "old"}}}},
		}
		if reverse {
			for i, j := 0, len(tasks)-1; i < j; i, j = i+1, j-1 {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
		svc.engine = &mockWorkflowEngine{execution: &domain.EngineExecution{Status: "RUNNING", Tasks: tasks}}
		if err := svc.reconcileConductor(context.Background(), job); err != nil {
			t.Fatal(err)
		}
		updated, _ := steps.Get(context.Background(), sr.ID)
		if updated.Status != domain.SRNeedConfirm || len(updated.Candidates) != 1 || updated.Candidates[0].Content != "new" {
			t.Fatal("old generation overwrote the current projection")
		}
	}
}

func TestCurrentApprovalIgnoresOldWaitingTaskWhenNewRoundStarted(t *testing.T) {
	svc, jobs, steps := newTestService(false)
	engine := &mockWorkflowEngine{execution: &domain.EngineExecution{Status: "RUNNING", Tasks: []domain.EngineTask{
		{ReferenceTaskName: "step_7331_approval__1", Status: "IN_PROGRESS"},
		{ReferenceTaskName: "step_7331_loop", Status: "IN_PROGRESS", Output: map[string]any{"iteration": float64(2)}},
	}}}
	svc.engine = engine
	jobs.jobs["job1"] = &domain.Job{ID: "job1", Status: domain.JobNeedConfirm, ConductorWorkflowID: "execution"}
	sr := makeClaimedStepRun(steps, false)
	sr.Status = domain.SRNeedConfirm
	if _, err := svc.Regenerate(context.Background(), sr.ID); err == nil || engine.completedRef != "" {
		t.Fatal("old waiting task was completed after a newer round began")
	}
}
