package job

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/executable"
)

type unavailableEditor struct{}

func (unavailableEditor) Get(context.Context, string) (*wfdomain.Workflow, error) {
	panic("retry consulted edited workflow")
}

type recordingRetrySteps struct {
	*mockStepRunRepo
	updates map[string]map[string]any
	fail    bool
}

func (r *recordingRetrySteps) UpdateStatus(ctx context.Context, id string, status domain.StepRunStatus, updates map[string]any) error {
	if r.fail {
		return errors.New("private database failure")
	}
	r.updates[id] = updates
	return r.mockStepRunRepo.UpdateStatus(ctx, id, status, updates)
}

func frozenRetryService(t *testing.T) (*Service, *mockJobRepo, *recordingRetrySteps, *retryCompilationStub, *mockWorkflowEngine) {
	t.Helper()
	svc, jobs, base := newTestService(false)
	svc.wfRepo = unavailableEditor{}
	step := func(key string, order int, parents ...string) executable.ExecutableStepIR {
		values := map[string]executable.ValueRef{}
		for _, parent := range parents {
			values[parent] = executable.ValueRef{Source: executable.ValueStepOutput, Ref: parent}
		}
		return executable.ExecutableStepIR{Key: key, Order: order, DependsOn: parents, Context: values}
	}
	ir := executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, WorkflowID: "wf1", WorkflowVersion: 1, OutputStepKey: "sink", Steps: []executable.ExecutableStepIR{step("root", 1), step("s1", 2, "root"), step("sibling", 3, "root"), step("middle", 4, "s1"), step("sink", 5, "middle", "sibling")}}
	encoded, _ := json.Marshal(ir)
	comp := &retryCompilationStub{item: &wfdomain.Compilation{WorkflowID: "wf1", WorkflowVersion: 1, DefinitionHash: "frozen", IRJSON: encoded}}
	svc.compilations = comp
	j := &domain.Job{ID: "job1", WorkflowID: "wf1", WorkflowVersion: 1, CompilationHash: "frozen", Origin: "native", Status: domain.JobFailed, ConductorWorkflowID: "execution"}
	jobs.jobs[j.ID] = j
	for _, key := range []string{"root", "s1", "sibling", "middle", "sink"} {
		status := domain.SRSkipped
		if key == "s1" {
			status = domain.SRFailed
		}
		if key == "root" || key == "sibling" {
			status = domain.SRSucceeded
		}
		base.items[key] = &domain.StepRun{ID: key, JobID: j.ID, StepKey: key, Status: status, OutputAssetID: "retained-" + key, Candidates: []domain.Candidate{{URL: "private"}}}
	}
	steps := &recordingRetrySteps{mockStepRunRepo: base, updates: map[string]map[string]any{}}
	svc.stepRunRepo = steps
	seq := 2
	engine := &mockWorkflowEngine{execution: &domain.EngineExecution{Status: "FAILED", Tasks: []domain.EngineTask{{TaskID: "render", Sequence: &seq, ReferenceTaskName: "step_7331_render__1", Status: "COMPLETED"}}}}
	svc.engine = engine
	return svc, jobs, steps, comp, engine
}

func TestStepRetryUsesFrozenTransitiveGraphWithoutReadingEditor(t *testing.T) {
	svc, jobs, steps, comp, engine := frozenRetryService(t)
	_, err := svc.RetryStepRun(context.Background(), "s1", "retry", nil)
	if err != nil || engine.rerunTaskID != "render" || comp.version != 1 {
		t.Fatal("frozen retry failed", err)
	}
	if len(steps.updates) != 3 {
		t.Fatal("retry did not reset exactly target and transitive descendants")
	}
	for _, key := range []string{"s1", "middle", "sink"} {
		updates, exists := steps.updates[key]
		if !exists || updates["output_asset_id"] != "" || steps.items[key].Status != domain.SRBlocked || len(steps.items[key].Candidates) != 0 {
			t.Fatal("transitive node was not invalidated")
		}
	}
	for _, key := range []string{"root", "sibling"} {
		if steps.items[key].Status != domain.SRSucceeded || steps.items[key].OutputAssetID != "retained-"+key || len(steps.items[key].Candidates) != 1 {
			t.Fatal("unaffected output or candidates changed")
		}
	}
	if jobs.jobs["job1"].RetryCount != 1 {
		t.Fatal("retry epoch not advanced")
	}
}

func TestFrozenRetryRejectsSourceMismatchBeforeEngineMutation(t *testing.T) {
	for _, mutation := range []string{"compilation_identity", "compilation_version", "hash", "ir_identity", "ir_version", "unknown_field", "missing_step", "duplicate_step"} {
		t.Run(mutation, func(t *testing.T) {
			svc, jobs, steps, comp, engine := frozenRetryService(t)
			switch mutation {
			case "compilation_identity":
				comp.item.WorkflowID = "other"
			case "compilation_version":
				comp.item.WorkflowVersion = 2
			case "hash":
				comp.item.DefinitionHash = "other"
			case "ir_identity", "ir_version", "unknown_field":
				var ir map[string]any
				_ = json.Unmarshal(comp.item.IRJSON, &ir)
				if mutation == "ir_identity" {
					ir["workflow_id"] = "other"
				} else if mutation == "ir_version" {
					ir["workflow_version"] = 2
				} else {
					ir["unknown"] = "private"
				}
				comp.item.IRJSON, _ = json.Marshal(ir)
			case "missing_step":
				delete(steps.items, "middle")
			case "duplicate_step":
				steps.items["middle"].StepKey = "sibling"
			}
			_, err := svc.RetryStepRun(context.Background(), "s1", "retry", nil)
			if err == nil || engine.rerunTaskID != "" || len(steps.updates) != 0 || jobs.jobs["job1"].RetryCount != 0 || steps.items["s1"].Status != domain.SRFailed {
				t.Fatal("mismatched frozen retry mutated execution")
			}
		})
	}
}

func TestRetryLocalFailureIsNotReportedAsSuccess(t *testing.T) {
	svc, _, steps, _, engine := frozenRetryService(t)
	steps.fail = true
	_, err := svc.RetryStepRun(context.Background(), "s1", "retry", nil)
	if err == nil || engine.rerunTaskID != "render" || err.Error() == "private database failure" {
		t.Fatal("local persistence failure hidden or exposed")
	}
}
