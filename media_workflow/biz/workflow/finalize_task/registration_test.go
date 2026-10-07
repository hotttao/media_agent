package finalize_task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/shared/taskcontext"
	"media_agent/media_workflow/biz/shared/taskerror"
	"media_agent/media_workflow/biz/shared/taskref"
)

type registrationJobs struct{ item *domain.Job }

func (r registrationJobs) Get(context.Context, string) (*domain.Job, error) { return r.item, nil }

type registrationSteps struct{ item *domain.StepRun }

func (r registrationSteps) GetByJobAndStepKey(context.Context, string, string) (*domain.StepRun, error) {
	return r.item, nil
}

type registrationExecution struct {
	item *domain.EngineExecution
	err  error
}

func (r *registrationExecution) Get(context.Context, string) (*domain.EngineExecution, error) {
	return r.item, r.err
}

type registrationAssets struct {
	keys []string
	ids  map[string]string
	lost bool
}

func (r *registrationAssets) UpsertGeneratedAsset(_ context.Context, request *client.UpsertGenAssetReq) (string, error) {
	r.keys = append(r.keys, request.ResultKey)
	id := r.ids[request.ResultKey]
	if id == "" {
		id = fmt.Sprintf("gen_%d", len(r.ids)+1)
		r.ids[request.ResultKey] = id
	}
	if r.lost {
		r.lost = false
		return "", errors.New("response lost after commit")
	}
	return id, nil
}

func registrationFixture() (Handler, context.Context, Input, *registrationExecution, *registrationAssets) {
	input := Input{JobID: "job", StepKey: "image", OutputAssetKind: "image", ApprovalPolicy: "when_multiple_candidates", Candidates: []Candidate{{Content: "fixture"}}}
	var raw map[string]any
	_ = decode(input, &raw)
	execution := &registrationExecution{item: &domain.EngineExecution{WorkflowInstanceID: "engine", Status: "RUNNING", Tasks: []domain.EngineTask{
		{TaskID: "finalize", TaskType: TaskDefinitionName, ReferenceTaskName: "step__image__finalize", Status: "IN_PROGRESS", Input: raw},
		{TaskID: "invoke", ReferenceTaskName: taskref.Reference("image", "invoke") + "__1", Status: "COMPLETED", Output: map[string]any{"candidates": input.Candidates}},
		{ReferenceTaskName: taskref.Reference("image", "loop"), Output: map[string]any{"iteration": 1}},
	}}}
	assets := &registrationAssets{ids: map[string]string{}}
	h := Handler{Jobs: registrationJobs{item: &domain.Job{ID: "job", OrganizationID: "org", Scope: "ai_clothing", SubmittedBy: "owner", ConductorWorkflowID: "engine", Status: domain.JobFailed,
		Input: domain.JobInput{Product: &domain.ProductData{ID: "product"}}}}, Steps: registrationSteps{item: &domain.StepRun{ID: "stable-step", JobID: "job", StepKey: "image", TemplateID: "template", TemplateVersion: 1, AssetKind: "image"}},
		Executions: execution, Assets: assets, StorageOrigin: "https://storage.test"}
	ctx := taskcontext.WithIdentity(context.Background(), taskcontext.Identity{WorkflowID: "engine", TaskID: "finalize", TaskType: TaskDefinitionName, Reference: "step__image__finalize"})
	return h, ctx, input, execution, assets
}

func TestFinalizeRegistrationUsesStableInvocationIdentityAcrossTechnicalRetry(t *testing.T) {
	h, ctx, input, execution, assets := registrationFixture()
	assets.lost = true
	if _, err := h.Execute(ctx, input); err == nil || taskerror.IsPermanent(err) {
		t.Fatal("lost registration response must be retryable", err)
	}
	// Conductor creates a new Finalize task while retaining the successful invoke.
	execution.item.Tasks[0].TaskID = "finalize-retry"
	ctx = taskcontext.WithIdentity(ctx, taskcontext.Identity{WorkflowID: "engine", TaskID: "finalize-retry", TaskType: TaskDefinitionName, Reference: "step__image__finalize"})
	out, err := h.Execute(ctx, input)
	want, _ := domain.GenerationResultKey("engine", "stable-step", "invoke")
	if err != nil || out.OutputResourceID != "gen_1" || out.OutputResource.ID != out.OutputResourceID || len(assets.ids) != 1 || len(assets.keys) != 2 || assets.keys[0] != want || assets.keys[1] != want {
		t.Fatal("Finalize retry created a new business asset or returned a placeholder", err)
	}
	execution.item.Tasks[1].TaskID = "new-invoke"
	newResult, err := h.Execute(ctx, input)
	if err != nil || newResult.OutputResourceID == out.OutputResourceID || len(assets.ids) != 2 {
		t.Fatal("new invocation reused an old result", err)
	}
}

func TestFinalizeRejectsStaleOrForeignFactsBeforeRegistration(t *testing.T) {
	for _, scenario := range []string{"no_identity", "wrong_execution", "cancelled", "wrong_step", "wrong_kind", "retired_finalize", "retired_invoke", "pending_generation", "old_candidates", "old_selected", "engine_terminated", "ambiguous_invoke"} {
		t.Run(scenario, func(t *testing.T) {
			h, ctx, input, execution, assets := registrationFixture()
			switch scenario {
			case "no_identity":
				ctx = context.Background()
			case "wrong_execution":
				h.Jobs.(registrationJobs).item.ConductorWorkflowID = "other"
			case "cancelled":
				h.Jobs.(registrationJobs).item.Status = domain.JobCancelled
			case "wrong_step":
				h.Steps.(registrationSteps).item.JobID = "other"
			case "wrong_kind":
				h.Steps.(registrationSteps).item.AssetKind = "video"
			case "retired_finalize":
				execution.item.Tasks[0].Retried = true
			case "retired_invoke":
				execution.item.Tasks[1].Retried = true
			case "pending_generation":
				execution.item.Tasks[2].Output["iteration"] = 2
			case "old_candidates":
				execution.item.Tasks[1].Output["candidates"] = []Candidate{{Content: "old"}}
			case "old_selected":
				input.SelectedCandidate = &Candidate{Content: "old"}
				_ = decode(input, &execution.item.Tasks[0].Input)
			case "engine_terminated":
				execution.item.Status = "TERMINATED"
			case "ambiguous_invoke":
				execution.item.Tasks = append(execution.item.Tasks, execution.item.Tasks[1])
			}
			if _, err := h.Execute(ctx, input); err == nil || !taskerror.IsPermanent(err) || len(assets.keys) != 0 {
				t.Fatal("invalid generation was registered or misclassified", err)
			}
		})
	}
}

func TestFinalizeMediaReferenceIsControlledAndSignatureFree(t *testing.T) {
	for _, raw := range []string{"https://storage.test/media/materials/org/ai_clothing/frame.png?signature=short-lived", "object_path:materials/org/ai_clothing/frame.png", ""} {
		h, ctx, input, execution, _ := registrationFixture()
		input.Candidates = []Candidate{{URL: raw}}
		if raw == "" {
			input.Candidates[0].ObjectPath = "materials/org/ai_clothing/frame.png"
		}
		_ = decode(input, &execution.item.Tasks[0].Input)
		execution.item.Tasks[1].Output["candidates"] = input.Candidates
		out, err := h.Execute(ctx, input)
		if err != nil || out.OutputResource.AssetRef != "object_path:materials/org/ai_clothing/frame.png" || strings.Contains(out.OutputResource.URL, "?") {
			t.Fatal("media was not normalized", err)
		}
	}
	for _, raw := range []string{"https://foreign.test/media/materials/org/ai_clothing/a.png", "object_path:materials/other/ai_clothing/a.png", "object_path:materials/org/other/a.png", "object_path:materials/org/ai_clothing/../a.png"} {
		h, ctx, input, execution, assets := registrationFixture()
		input.Candidates = []Candidate{{URL: raw}}
		_ = decode(input, &execution.item.Tasks[0].Input)
		execution.item.Tasks[1].Output["candidates"] = input.Candidates
		if _, err := h.Execute(ctx, input); err == nil || len(assets.keys) != 0 {
			t.Fatal("foreign media registered")
		}
	}
}

func TestFinalizeRequiresMatchingCurrentHumanApproval(t *testing.T) {
	h, ctx, input, execution, assets := registrationFixture()
	input.ApprovalPolicy = "always"
	input.SelectedCandidate = &input.Candidates[0]
	_ = decode(input, &execution.item.Tasks[0].Input)
	if _, err := h.Execute(ctx, input); err == nil || len(assets.keys) != 0 {
		t.Fatal("missing HUMAN accepted")
	}
	execution.item.Tasks = append(execution.item.Tasks, domain.EngineTask{TaskID: "human", TaskType: "HUMAN", ReferenceTaskName: taskref.Reference("image", "approval") + "__1", Status: "COMPLETED", Output: map[string]any{"decision": "accept", "selected_candidate": *input.SelectedCandidate}})
	if _, err := h.Execute(ctx, input); err != nil || len(assets.keys) != 1 {
		t.Fatal("valid HUMAN rejected", err)
	}
	execution.item.Tasks[3].Output["decision"] = "regenerate"
	if _, err := h.Execute(ctx, input); err == nil || len(assets.keys) != 1 {
		t.Fatal("regeneration decision registered a result")
	}
}
