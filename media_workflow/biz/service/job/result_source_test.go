package job

import (
	"context"
	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/taskref"
	"testing"
)

func TestInternalReusePreparationFailureIdentifiesTheActualNode(t *testing.T) {
	for _, phase := range []string{"prepare", "reuse"} {
		svc, jobs, _, engine, sr := projectionFixture()
		engine.execution.Status = "FAILED"
		engine.execution.Tasks = []domain.EngineTask{{ReferenceTaskName: taskref.Reference(sr.StepKey, phase), Status: "FAILED_WITH_TERMINAL_ERROR"}}
		if err := svc.reconcileConductor(context.Background(), jobs.jobs[sr.JobID]); err != nil {
			t.Fatal(err)
		}
		if sr.Status != domain.SRFailed || jobs.jobs[sr.JobID].Status != domain.JobFailed {
			t.Fatal("internal task failure remained a blocked node", phase)
		}
	}
}

func TestResultSourcesRequireCurrentCompletedMatchingOutput(t *testing.T) {
	for _, mode := range []string{"reused", "generated", "legacy", "pending", "retried", "wrong_asset", "wrong_engine", "cancelled", "missing_source"} {
		t.Run(mode, func(t *testing.T) {
			j := &domain.Job{ConductorWorkflowID: "engine", Status: domain.JobSucceeded}
			sr := &domain.StepRun{StepKey: "image", Status: domain.SRSucceeded, OutputAssetID: "gen_accepted", ResultSource: "old", ReusedFromStepRunID: "old"}
			task := domain.EngineTask{ReferenceTaskName: "step__image__finalize", Status: "COMPLETED", Output: map[string]any{"output_resource_id": "gen_accepted", "result_source": "reused", "reused_from_step_run_id": "source"}}
			snapshot := &domain.EngineExecution{WorkflowInstanceID: "engine"}
			switch mode {
			case "generated":
				task.Output["result_source"], task.Output["reused_from_step_run_id"] = "generated", ""
			case "legacy":
				delete(task.Output, "result_source")
			case "pending":
				task.Status = "IN_PROGRESS"
			case "retried":
				task.Retried = true
			case "wrong_asset":
				task.Output["output_resource_id"] = "gen_other"
			case "wrong_engine":
				snapshot.WorkflowInstanceID = "other"
			case "cancelled":
				j.Status = domain.JobCancelled
			case "missing_source":
				delete(task.Output, "reused_from_step_run_id")
			}
			snapshot.Tasks = []domain.EngineTask{task}
			annotateResultSources(snapshot, j, []*domain.StepRun{sr})
			want, from := "", ""
			if mode == "reused" {
				want, from = "reused", "source"
			}
			if mode == "generated" {
				want = "generated"
			}
			if sr.ResultSource != want || sr.ReusedFromStepRunID != from {
				t.Fatalf("untrusted or stale provenance displayed: %+v", sr)
			}
		})
	}
}
