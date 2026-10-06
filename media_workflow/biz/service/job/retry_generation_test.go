package job

import (
	"context"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
)

func TestStepRetryTargetsLatestRenderRegardlessOfTaskOrder(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		svc, jobs, steps := newTestService(false)
		sr := makeClaimedStepRun(steps, false)
		sr.Status = domain.SRFailed
		jobs.jobs["job1"] = &domain.Job{ID: "job1", WorkflowID: "wf1", Status: domain.JobFailed, ConductorWorkflowID: "execution"}
		jobs.stepRuns["job1"] = []*domain.StepRun{sr}
		sequence := 2
		tasks := []domain.EngineTask{
			{TaskID: "first-render", ReferenceTaskName: "step_7331_render__1", Status: "COMPLETED"},
			{TaskID: "second-render", Sequence: &sequence, ReferenceTaskName: "step_7331_render__2", Status: "COMPLETED"},
			{TaskID: "failed-invoke", ReferenceTaskName: "step_7331_invoke__2", Status: "FAILED"},
			{TaskID: "loop", ReferenceTaskName: "step_7331_loop", Output: map[string]any{"iteration": float64(2)}},
		}
		if reverse {
			for i, j := 0, len(tasks)-1; i < j; i, j = i+1, j-1 {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
		engine := &mockWorkflowEngine{execution: &domain.EngineExecution{Status: "FAILED", Tasks: tasks}}
		svc.engine = engine
		if _, err := svc.RetryStepRun(context.Background(), sr.ID, "retry", nil); err != nil {
			t.Fatal(err)
		}
		if engine.rerunTaskID != "second-render" {
			t.Fatal("retry targeted older generation")
		}
	}
}

func TestStepRetryRejectsNonfailedOrMissingLatestGeneration(t *testing.T) {
	for _, kind := range []string{"running", "cancelled", "missing latest", "ambiguous", "max retries"} {
		t.Run(kind, func(t *testing.T) {
			svc, jobs, steps := newTestService(false)
			sr := makeClaimedStepRun(steps, false)
			sr.Status = domain.SRFailed
			j := &domain.Job{ID: "job1", Status: domain.JobFailed, ConductorWorkflowID: "execution"}
			jobs.jobs[j.ID] = j
			execution := &domain.EngineExecution{Status: "FAILED", Tasks: []domain.EngineTask{{TaskID: "render", ReferenceTaskName: "step_7331_render__2", Status: "COMPLETED"}}}
			switch kind {
			case "running":
				execution.Status = "RUNNING"
			case "cancelled":
				j.Status = domain.JobCancelled
			case "missing latest":
				execution.Tasks = append(execution.Tasks, domain.EngineTask{ReferenceTaskName: "step_7331_loop", Output: map[string]any{"iteration": float64(3)}})
			case "ambiguous":
				execution.Tasks = append(execution.Tasks, execution.Tasks[0])
			case "max retries":
				sr.RetryCount = 3
			}
			engine := &mockWorkflowEngine{execution: execution}
			svc.engine = engine
			if _, err := svc.RetryStepRun(context.Background(), sr.ID, "retry", nil); err == nil || engine.rerunTaskID != "" || sr.Status != domain.SRFailed {
				t.Fatal("unsafe retry was accepted", err)
			}
		})
	}
}
