package publication

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	capability "media_agent/media_workflow/biz/domain/capability"
	domain "media_agent/media_workflow/biz/domain/workflow"
)

type publicationTemplates struct {
	calls       int
	unavailable bool
}

func (r *publicationTemplates) GetVersion(_ context.Context, id string, version int) (*domain.TemplateSnapshot, error) {
	r.calls++
	if r.unavailable || id != "tpl" || version != 1 {
		return nil, errors.New("template unavailable")
	}
	return &domain.TemplateSnapshot{ID: "tpl", Scope: "e2e", Status: "published", PromptBody: "frozen prompt {{.description}}", OutputAssetKind: "image", CandidateCount: 1,
		CapabilityKey: "cap", CapabilityVersion: 1, Inputs: []domain.SlotSnapshot{{Name: "description", ResourceType: "TEXT", Required: true}},
		InvocationMapping: map[string]domain.InvocationExpressionSnapshot{"prompt": {Op: "render_template"}}}, nil
}

type publicationCapabilities struct {
	capability.Repo
	calls int
}

func (r *publicationCapabilities) Get(context.Context, string, int) (*capability.Capability, error) {
	r.calls++
	return &capability.Capability{Key: "cap", Version: 1, ConductorTaskDef: "cap_provider_v1", OutputAssetKind: "image", ExecutionTarget: capability.ExecutionTargetPersonalWorker,
		Inputs: []capability.Input{{Name: "prompt", Type: capability.InputString, Required: true}}, RetryCount: 4, RetryLogic: "FIXED", RetryDelaySeconds: 2, TimeoutSeconds: 120, Status: "published"}, nil
}

func TestAcceptPublicationFreezesAuthoritativeSourceAndReplaysOffline(t *testing.T) {
	tx := &publicationTx{}
	source := &publicationSource{source: &domain.Workflow{ID: "wf", Scope: "e2e", Version: 1, CreatedBy: "owner", Status: domain.StatusDraft, OutputStepKey: "image",
		Steps: []domain.Step{{Key: "image", Order: 1, TemplateID: "tpl", TemplateVersion: 1, ApprovalPolicy: domain.ApprovalWhenMultipleCandidates, ReusePolicy: domain.ReuseIfAvailable,
			SlotBindings: []domain.SlotBinding{{ToInput: "description", Source: domain.SourceWorkflowInput, Ref: "description"}}}}}}
	store := &publicationStore{source: source, tx: tx}
	templates := &publicationTemplates{}
	capabilities := &publicationCapabilities{}
	flow := &Flow{Workflows: source, Intents: store, Tx: tx, Templates: templates, Capabilities: capabilities}
	caller := *source.source
	caller.Steps = append([]domain.Step(nil), source.source.Steps...)
	caller.Steps[0].TemplateID = "caller-supplied-not-authoritative"
	accepted, err := flow.AcceptPublication(context.Background(), &caller, "owner")
	if err != nil {
		t.Fatal("acceptance failed", err)
	}
	if accepted.Validate() != nil || accepted.Status != "pending" || store.saves != 1 || tx.commits != 1 || templates.calls != 1 || capabilities.calls != 1 || !strings.Contains(string(accepted.DefinitionJSON), "frozen prompt") {
		t.Fatal("authoritative contract was not frozen")
	}
	var tasks []struct {
		Name    string `json:"name"`
		Retries int    `json:"retryCount"`
		Timeout int    `json:"timeoutSeconds"`
	}
	if json.Unmarshal(accepted.TaskDefinitionsJSON, &tasks) != nil || len(tasks) != 4 {
		t.Fatal("complete task definition set was not frozen")
	}
	names := make([]string, 0, len(tasks))
	providerFound := false
	for _, task := range tasks {
		names = append(names, task.Name)
		if task.Name == "cap_provider_v1" {
			providerFound = task.Retries == 4 && task.Timeout == 120
		}
	}
	if !sort.StringsAreSorted(names) || !providerFound {
		t.Fatal("task order or capability retry/timeout policy lost")
	}
	digest := accepted.PayloadDigest
	templates.unavailable = true
	replay, err := flow.AcceptPublication(context.Background(), &caller, "owner")
	if err != nil || replay.ID != accepted.ID || replay.PayloadDigest != digest || templates.calls != 1 || capabilities.calls != 1 || store.saves != 1 {
		t.Fatal("replay recompiled or changed accepted intent")
	}
	source.source.CreatedBy = "new-owner"
	if _, err := flow.AcceptPublication(context.Background(), &caller, "owner"); !errors.Is(err, domain.ErrPublicationConflict) {
		t.Fatal("replay bypassed current ownership")
	}
}
