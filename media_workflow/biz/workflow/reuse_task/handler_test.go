package reuse_task

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/shared/taskcontext"
	"media_agent/media_workflow/biz/shared/taskerror"
	"media_agent/media_workflow/biz/shared/taskref"
)

type checkJobs struct{ job *domain.Job }

func (r checkJobs) Get(context.Context, string) (*domain.Job, error) { return r.job, nil }

type checkSteps struct{ step *domain.StepRun }

func (r checkSteps) GetByJobAndStepKey(context.Context, string, string) (*domain.StepRun, error) {
	return r.step, nil
}

type checkExecution struct{ execution *domain.EngineExecution }

func (r checkExecution) Get(context.Context, string) (*domain.EngineExecution, error) {
	return r.execution, nil
}

type checkEvidence struct {
	items []domain.ReuseEvidence
	finds int
	err   error
}

func (r *checkEvidence) Find(_ context.Context, boundary domain.ReuseScope, fingerprint string) ([]domain.ReuseEvidence, error) {
	r.finds++
	if len(r.items) > 0 && (r.items[0].Boundary != boundary || r.items[0].Fingerprint != fingerprint) {
		return nil, domain.ErrReuseEvidenceConflict
	}
	return r.items, r.err
}
func (*checkEvidence) Save(context.Context, domain.ReuseEvidence) error {
	return errors.New("check task must never save evidence")
}

type checkAssets struct {
	asset *client.ReusableGeneratedAsset
	err   error
	calls int
}

func (r *checkAssets) VerifyReusableGeneratedAsset(context.Context, string, string, string, string) (*client.ReusableGeneratedAsset, error) {
	r.calls++
	return r.asset, r.err
}
func checkFixture(t *testing.T) (Handler, context.Context, Input, *checkExecution, *checkEvidence, *checkAssets) {
	t.Helper()
	identity := fingerprintIdentity()
	input := Input{JobID: "job", StepKey: "image", Policy: "reuse_if_available", Identity: identity, ProviderInput: map[string]any{"prompt": "dress", "images": []any{"person", "garment"}}}
	j := &domain.Job{ID: "job", Origin: "native", OrganizationID: "org", WorkspaceID: "workspace", Scope: "media", SubmittedBy: "owner", Status: domain.JobRunning, ConductorWorkflowID: "engine"}
	step := &domain.StepRun{ID: "step", JobID: j.ID, StepKey: input.StepKey, TemplateID: identity.TemplateID, TemplateVersion: identity.TemplateVersion, AssetKind: identity.OutputAssetKind}
	context := taskcontext.WithIdentity(context.Background(), taskcontext.Identity{WorkflowID: "engine", TaskID: "check", TaskType: TaskDefinitionName, Reference: taskref.Reference(input.StepKey, "reuse")})
	data, _ := json.Marshal(input)
	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	engine := &checkExecution{execution: &domain.EngineExecution{WorkflowInstanceID: "engine", Status: "RUNNING", Tasks: []domain.EngineTask{{TaskID: "check", TaskType: TaskDefinitionName, ReferenceTaskName: taskref.Reference(input.StepKey, "reuse"), Status: "IN_PROGRESS", Input: raw}}}}
	fingerprint, err := BuildFingerprint(context, j.OrganizationID, identity, input.ProviderInput, digestResolver{})
	if err != nil {
		t.Fatal(err)
	}
	entry := domain.ReuseEvidence{ID: domain.ReuseEvidenceID("source-step", "result"), Boundary: domain.ReuseScope{OrganizationID: j.OrganizationID, WorkspaceID: j.WorkspaceID, OwnerUserID: j.SubmittedBy, Scope: j.Scope}, Fingerprint: fingerprint, SourceJobID: "source-job", SourceStepRunID: "source-step", SourceResultKey: "result", AssetID: "gen_source", ObjectPath: "materials/org/media/source.png", AssetKind: identity.OutputAssetKind}
	evidence := &checkEvidence{items: []domain.ReuseEvidence{entry}}
	assets := &checkAssets{asset: &client.ReusableGeneratedAsset{ID: entry.AssetID, OrganizationID: j.OrganizationID, Scope: j.Scope, OwnerUserID: j.SubmittedBy, JobID: entry.SourceJobID, StepRunID: entry.SourceStepRunID, TemplateID: identity.TemplateID, TemplateVersion: identity.TemplateVersion, AssetKind: identity.OutputAssetKind, ObjectPath: entry.ObjectPath}}
	h := Handler{Jobs: checkJobs{j}, Steps: checkSteps{step}, Executions: engine, Evidence: evidence, Assets: assets, Digests: digestResolver{}, StorageOrigin: "https://storage.test"}
	return h, context, input, engine, evidence, assets
}
func TestReuseCheckReturnsOnlyVerifiedAcceptedResource(t *testing.T) {
	h, ctx, input, _, evidence, assets := checkFixture(t)
	output, err := h.Execute(ctx, input)
	if err != nil || !output.Hit || output.Resource.ID != "gen_source" || output.Resource.AssetRef != "object_path:materials/org/media/source.png" || output.SourceStepRunID != "source-step" || evidence.finds != 1 || assets.calls != 1 {
		t.Fatal("verified accepted result did not produce a reuse branch", err)
	}
}

func TestReuseCheckDefersUntilInitialExecutionBindingIsStored(t *testing.T) {
	h, ctx, input, _, evidence, assets := checkFixture(t)
	h.Jobs.(checkJobs).job.ConductorWorkflowID = ""
	if _, err := h.Execute(ctx, input); err == nil || taskerror.IsPermanent(err) || evidence.finds != 0 || assets.calls != 0 {
		t.Fatal("initial start binding race was treated as an invalid task or source lookup", err)
	}
}
func TestReuseForceNewNeverLooksUpOldResult(t *testing.T) {
	h, ctx, input, engine, evidence, assets := checkFixture(t)
	input.Policy = "force_new"
	engine.execution.Tasks[0].Input["policy"] = "force_new"
	output, err := h.Execute(ctx, input)
	if err != nil || output.Hit || output.Fingerprint == "" || evidence.finds != 0 || assets.calls != 0 {
		t.Fatal("force_new consulted a prior result", err)
	}
}
func TestReuseCheckDoesNotTreatStorageOutageAsMiss(t *testing.T) {
	h, ctx, input, _, _, assets := checkFixture(t)
	assets.err = client.ErrReusableAssetNotFound
	if output, err := h.Execute(ctx, input); err != nil || output.Hit {
		t.Fatal("removed source should allow generation", err)
	}
	assets.err = errors.New("verification temporarily unavailable")
	if _, err := h.Execute(ctx, input); err == nil || taskerror.IsPermanent(err) {
		t.Fatal("verification outage did not use native technical retry")
	}
}
func TestReuseCheckRejectsOldOrAlteredNativeTask(t *testing.T) {
	for _, mutate := range []func(*domain.EngineTask){func(t *domain.EngineTask) { t.Retried = true }, func(t *domain.EngineTask) { t.Status = "COMPLETED" }, func(t *domain.EngineTask) { t.TaskID = "other" }, func(t *domain.EngineTask) { t.Input["policy"] = "force_new" }} {
		h, ctx, input, engine, evidence, assets := checkFixture(t)
		mutate(&engine.execution.Tasks[0])
		if _, err := h.Execute(ctx, input); !taskerror.IsPermanent(err) || evidence.finds != 0 || assets.calls != 0 {
			t.Fatal("invalid native identity consulted reuse resources", err)
		}
	}
}
