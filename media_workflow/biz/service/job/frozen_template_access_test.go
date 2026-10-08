package job

import (
	"context"
	"encoding/json"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/executable"
	"testing"
)

type forbiddenTemplateLookup struct{ t *testing.T }

func (p forbiddenTemplateLookup) GetVersion(context.Context, string, int) (*wfdomain.TemplateSnapshot, error) {
	p.t.Fatal("published execution read private source template")
	return nil, nil
}

func TestSharedPublishedSubmitUsesFrozenCompilationNotTemplateACL(t *testing.T) {
	svc, jobs, starts, _, _, cmd := submissionFixture()
	wf := svc.wfRepo.(*mockWorkflowLookup).wf
	svc.tplLookup = forbiddenTemplateLookup{t}
	svc.ConfigureWorkflowAccess(func(_ context.Context, id, org, workspace, user string) error {
		if id != wf.ID || org != "org" || workspace != "workspace" || user != cmd.SubmittedBy {
			t.Fatal("definition authorization context mismatch")
		}
		return nil
	})
	ir := executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, WorkflowID: wf.ID, WorkflowVersion: wf.Version, OutputStepKey: wf.OutputStepKey}
	for _, step := range wf.Steps {
		ir.Steps = append(ir.Steps, executable.ExecutableStepIR{Key: step.Key, Template: executable.TemplateIR{ID: step.TemplateID, Version: step.TemplateVersion}, OutputAssetKind: "first_frame", CandidateCount: 1, Capability: executable.CapabilityIR{ConductorTaskDef: "jimeng_image", ExecutionTarget: "personal_worker", Provider: "jimeng", GenerationMethod: "i2i", GenerationType: "multi_image_prompt"}})
	}
	encoded, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	svc.compilations.(*retryCompilationStub).item.IRJSON = encoded
	job, err := svc.Submit(context.Background(), cmd)
	if err != nil || job == nil || starts.command == nil || len(jobs.jobs) != 1 {
		t.Fatal("frozen definition execution rejected", err)
	}
	for _, step := range jobs.stepRuns[job.ID] {
		if step.Platform != "jimeng" || step.AssetKind != "first_frame" {
			t.Fatal("frozen metadata not projected")
		}
	}
}
