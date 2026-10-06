package job

import (
	"context"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
)

func TestStepRetryRejectsCollateralBranchBeforeAnyMutation(t *testing.T) {
	for _, kind := range []string{"completed after", "failed before", "waiting before", "sequence missing", "unknown capability", "target sequence missing"} {
		t.Run(kind, func(t *testing.T) {
			svc, jobs, steps := newTestService(false)
			sr := makeClaimedStepRun(steps, false)
			sr.Status = domain.SRFailed
			j := &domain.Job{ID: "job1", Status: domain.JobFailed, ConductorWorkflowID: "execution"}
			jobs.jobs[j.ID] = j
			targetSeq, foreignSeq := 2, 3
			target := domain.EngineTask{TaskID: "render", Sequence: &targetSeq, ReferenceTaskName: "step_7331_render__1", Status: "COMPLETED"}
			other := domain.EngineTask{TaskID: "other", Sequence: &foreignSeq, ReferenceTaskName: "step_7332_invoke__1", Status: "COMPLETED", TaskType: "SIMPLE"}
			switch kind {
			case "failed before":
				foreignSeq, other.Status = 1, "FAILED"
			case "waiting before":
				foreignSeq, other.Status = 1, "IN_PROGRESS"
			case "sequence missing":
				other.Sequence = nil
			case "unknown capability":
				other.ReferenceTaskName = "unexpected"
			case "target sequence missing":
				target.Sequence = nil
			}
			engine := &mockWorkflowEngine{execution: &domain.EngineExecution{Status: "FAILED", Tasks: []domain.EngineTask{other, target}}}
			svc.engine = engine
			if _, err := svc.RetryStepRun(context.Background(), sr.ID, "retry", nil); err == nil || engine.rerunTaskID != "" || sr.Status != domain.SRFailed || j.Status != domain.JobFailed || sr.RetryCount != 0 || len(j.RetryHistory) != 0 {
				t.Fatal("collateral rerun accepted or mutated business state")
			}
		})
	}
}

func TestStepRetryRetainsCompletedPredecessor(t *testing.T) {
	zero, two := 0, 2
	e := &domain.EngineExecution{Tasks: []domain.EngineTask{
		{TaskID: "render", Sequence: &two, ReferenceTaskName: "step_7331_render__1", Status: "COMPLETED"},
		{TaskID: "previous", Sequence: &zero, ReferenceTaskName: "step__s2__finalize", Status: "COMPLETED"},
	}}
	if err := validateStepRerunIsolation(e, "s1", "render"); err != nil {
		t.Fatal(err)
	}
}
