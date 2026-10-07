package job

import (
	"context"
	"encoding/json"
	"go.opentelemetry.io/otel/propagation"
	"media_agent/hertz_infra/tracecontext"
	"media_agent/media_workflow/biz/shared/taskcontext"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/executable"
)

type retryCompilationStub struct {
	wfdomain.CompilationRepo
	item    *wfdomain.Compilation
	version int
}

func (r *retryCompilationStub) GetRegistered(_ context.Context, _ string, version int) (*wfdomain.Compilation, error) {
	r.version = version
	return r.item, nil
}

func TestRetryOverrideUsesFrozenSchemaAndStableStorage(t *testing.T) {
	svc, repo, steps := newTestService(false)
	const originalTrace = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	engine := &mockWorkflowEngine{execution: &domain.EngineExecution{WorkflowInstanceID: "execution-1", Status: "FAILED", Input: map[string]any{taskcontext.TraceParentKey: originalTrace}}}
	svc.engine = engine
	svc.tx = commandReviewTx{}
	// The editor has changed since this Job's publication. It must not decide
	// which inputs a frozen execution accepts.
	svc.wfRepo.(*mockWorkflowLookup).wf.InputSchema = []wfdomain.InputSchemaEntry{{Name: "new_input", ResourceType: "MATERIAL", Required: true}}
	ir := executable.ExecutableWorkflowIR{WorkflowID: "wf1", WorkflowVersion: 2, Steps: []executable.ExecutableStepIR{{Template: executable.TemplateIR{ID: "tpl1", Version: 1}, Context: map[string]executable.ValueRef{"scene": {Source: executable.ValueWorkflowInput, Ref: "scene"}}}}}
	encoded, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	compilation := &retryCompilationStub{item: &wfdomain.Compilation{IRJSON: encoded, DefinitionHash: "frozen-hash"}}
	svc.compilations = compilation
	job := &domain.Job{ID: "job1", OrganizationID: "org", WorkflowID: "wf1", WorkflowVersion: 2, CompilationHash: "frozen-hash", Status: domain.JobFailed, ConductorWorkflowID: "execution-1"}
	repo.jobs[job.ID] = job
	step := makeClaimedStepRun(steps, false)
	step.Status = domain.SRFailed
	raw := "https://example.test/media/materials/org/new.png?signature=temporary"
	input := domain.JobInput{Materials: []domain.MaterialData{{Name: "scene", URL: raw}}}
	requestCtx := tracecontext.Extract(context.Background(), propagation.MapCarrier{"traceparent": "00-11111111111111111111111111111111-2222222222222222-00"})
	updated, err := svc.Retry(requestCtx, job.ID, "new input", &input)
	if err != nil {
		t.Fatal(err)
	}
	want := "object_path:materials/org/new.png"
	if compilation.version != 2 || updated.Input.Materials[0].URL != want || updated.RetryHistory[0].InputOverride.Materials[0].URL != want || input.Materials[0].URL != raw {
		t.Fatal("retry lost frozen version, persisted signatures, or modified caller input")
	}
	inputs := engine.rerunInput["inputs"].(map[string]any)
	if inputs["scene"].(map[string]any)["asset_ref"] != want {
		t.Fatal("Conductor rerun received temporary URL")
	}
	if engine.rerunInput[taskcontext.TraceParentKey] != originalTrace {
		t.Fatal("whole rerun replaced original execution trace with request trace")
	}
}

func TestRetryInvalidMediaDoesNotChangeJobOrRestartEngine(t *testing.T) {
	for _, tc := range []struct{ name, inputName, url string }{
		{"foreign enterprise", "scene", "https://example.test/media/materials/foreign/image.png"},
		{"uncontrolled origin", "scene", "https://other.test/media/materials/org/image.png"},
		{"missing required input", "other", "https://example.test/media/materials/org/image.png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, steps := newTestService(false)
			engine := &mockWorkflowEngine{execution: &domain.EngineExecution{WorkflowInstanceID: "execution-1", Status: "FAILED"}}
			svc.engine = engine
			svc.tx = commandReviewTx{}
			ir := executable.ExecutableWorkflowIR{WorkflowID: "wf1", WorkflowVersion: 1, Steps: []executable.ExecutableStepIR{{Template: executable.TemplateIR{ID: "tpl1", Version: 1}, Context: map[string]executable.ValueRef{"scene": {Source: executable.ValueWorkflowInput, Ref: "scene"}}}}}
			encoded, _ := json.Marshal(ir)
			svc.compilations = &retryCompilationStub{item: &wfdomain.Compilation{IRJSON: encoded, DefinitionHash: "frozen-hash"}}
			job := &domain.Job{ID: "job1", OrganizationID: "org", WorkflowID: "wf1", WorkflowVersion: 1, CompilationHash: "frozen-hash", Status: domain.JobFailed, ConductorWorkflowID: "execution-1"}
			repo.jobs[job.ID] = job
			step := makeClaimedStepRun(steps, false)
			step.Status = domain.SRFailed
			input := domain.JobInput{Materials: []domain.MaterialData{{Name: tc.inputName, URL: tc.url}}}
			if got, err := svc.Retry(context.Background(), job.ID, "retry", &input); err == nil || got != nil || engine.rerunInput != nil || job.Status != domain.JobFailed || step.Status != domain.SRFailed || len(job.RetryHistory) != 0 {
				t.Fatal("invalid retry changed execution state")
			}
		})
	}
}
