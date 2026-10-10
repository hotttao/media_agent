package job

import (
	"context"
	"errors"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
)

type countedProjectionEngine struct {
	domain.WorkflowEngine
	calls int
}

func (e *countedProjectionEngine) Get(ctx context.Context, id string) (*domain.EngineExecution, error) {
	e.calls++
	return e.WorkflowEngine.Get(ctx, id)
}

func TestCompletedStepSkipsNoOpPhasesButStillReadsEngine(t *testing.T) {
	svc, jobs, steps, engine, sr := projectionFixture()
	sr.Status, sr.OutputAssetID = domain.SRSucceeded, "gen_generated"
	engine.execution.Tasks = append(engine.execution.Tasks, domain.EngineTask{ReferenceTaskName: "step__s1__render", Status: "COMPLETED", Output: map[string]any{"prompt": "rendered"}})
	engine.execution.Tasks[1].Output = map[string]any{"candidates": []domain.Candidate{{URL: "https://example.test/output"}}}
	counted := &countedProjectionEngine{WorkflowEngine: engine}
	svc.engine = counted
	tx := &startTxProbe{}
	svc.tx = tx
	svc.stepRunRepo = &projectionStepRepo{mockStepRunRepo: steps, inputsErr: errors.New("must not rewrite completed inputs"), candidatesErr: errors.New("must not rewrite completed candidates")}
	// Do not configure native review generation facts: this test verifies the
	// no-op phase boundary; other tests exercise the real review guards.
	svc.reviews = nil
	if err := svc.reconcileConductor(t.Context(), jobs.jobs[sr.JobID]); err != nil {
		t.Fatal(err)
	}
	if tx.calls != 1 {
		t.Fatalf("completed read used %d projection transactions, want only the aggregate guard", tx.calls)
	}
	engine.execution.Status = "PAUSED"
	if err := svc.reconcileConductor(t.Context(), jobs.jobs[sr.JobID]); err != nil {
		t.Fatal(err)
	}
	if counted.calls != 2 || tx.calls != 2 || jobs.jobs[sr.JobID].Status != domain.JobPaused || sr.OutputAssetID != "gen_generated" {
		t.Fatal("optimization cached engine state or changed completed output")
	}
}
