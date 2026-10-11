package reuse_task

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/shared/taskcontext"
	"media_agent/media_workflow/biz/shared/taskerror"
	"media_agent/media_workflow/biz/shared/taskref"
	"media_agent/media_workflow/biz/workflow/executable"
)

const TaskDefinitionName = "media_check_result_reuse_v1"

type Input struct {
	JobID         string         `json:"job_id"`
	StepKey       string         `json:"step_key"`
	Policy        string         `json:"policy"`
	Identity      Identity       `json:"identity"`
	ProviderInput map[string]any `json:"provider_input"`
}
type Output struct {
	Hit             bool                    `json:"hit"`
	Fingerprint     string                  `json:"fingerprint,omitempty"`
	EvidenceID      string                  `json:"evidence_id,omitempty"`
	SourceStepRunID string                  `json:"source_step_run_id,omitempty"`
	Resource        executable.StepResource `json:"resource"`
}
type JobLookup interface {
	Get(context.Context, string) (*domain.Job, error)
}
type StepLookup interface {
	GetByJobAndStepKey(context.Context, string, string) (*domain.StepRun, error)
}
type ExecutionLookup interface {
	Get(context.Context, string) (*domain.EngineExecution, error)
}
type AssetLookup interface {
	VerifyReusableGeneratedAsset(context.Context, string, string, string, string) (*client.ReusableGeneratedAsset, error)
}
type Handler struct {
	Jobs          JobLookup
	Steps         StepLookup
	Executions    ExecutionLookup
	Evidence      domain.ResultReuseRepo
	Assets        AssetLookup
	Digests       MediaDigestResolver
	StorageOrigin string
	StorageBucket string
}

func (h Handler) Execute(ctx context.Context, input Input) (Output, error) {
	id, ok := taskcontext.FromContext(ctx)
	if !ok || id.TaskType != TaskDefinitionName || input.JobID == "" || input.StepKey == "" || id.Reference != taskref.Reference(input.StepKey, "reuse") || (input.Policy != "reuse_if_available" && input.Policy != "force_new") {
		return Output{}, taskerror.Permanent("reuse_task_identity_invalid")
	}
	if h.Jobs == nil || h.Steps == nil || h.Executions == nil || h.Evidence == nil || h.Assets == nil {
		return Output{}, taskerror.Retryable("reuse_dependencies_unavailable")
	}
	j, err := h.Jobs.Get(ctx, input.JobID)
	if err != nil || j == nil {
		return Output{}, taskerror.Retryable("reuse_job_unavailable")
	}
	boundary := domain.ReuseScope{OrganizationID: j.OrganizationID, WorkspaceID: j.WorkspaceID, OwnerUserID: j.SubmittedBy, Scope: j.Scope}
	if j.ConductorWorkflowID == "" {
		return Output{}, taskerror.Retryable("reuse_execution_not_bound")
	}
	if !boundary.Valid() || j.Origin != "native" || j.ConductorWorkflowID != id.WorkflowID || j.Status == domain.JobCancelled {
		return Output{}, taskerror.Permanent("reuse_job_identity_invalid")
	}
	step, err := h.Steps.GetByJobAndStepKey(ctx, j.ID, input.StepKey)
	if err != nil || step == nil {
		return Output{}, taskerror.Retryable("reuse_step_unavailable")
	}
	if step.JobID != j.ID || step.StepKey != input.StepKey || step.TemplateID != input.Identity.TemplateID || step.TemplateVersion != input.Identity.TemplateVersion || step.AssetKind != input.Identity.OutputAssetKind {
		return Output{}, taskerror.Permanent("reuse_step_identity_invalid")
	}
	execution, err := h.Executions.Get(ctx, id.WorkflowID)
	if err != nil || execution == nil {
		return Output{}, taskerror.Retryable("reuse_execution_unavailable")
	}
	if execution.WorkflowInstanceID != id.WorkflowID || execution.Status != "RUNNING" {
		return Output{}, taskerror.Permanent("reuse_execution_inactive")
	}
	var native *domain.EngineTask
	for i := range execution.Tasks {
		if execution.Tasks[i].TaskID == id.TaskID {
			if native != nil {
				return Output{}, taskerror.Permanent("reuse_task_ambiguous")
			}
			native = &execution.Tasks[i]
		}
	}
	if native == nil || native.Retried || native.TaskType != TaskDefinitionName || native.ReferenceTaskName != id.Reference || (native.Status != "IN_PROGRESS" && native.Status != "SCHEDULED") {
		return Output{}, taskerror.Permanent("reuse_task_inactive")
	}
	var frozen Input
	payload, err := json.Marshal(native.Input)
	if err != nil || json.Unmarshal(payload, &frozen) != nil || !equalInput(frozen, input) {
		return Output{}, taskerror.Permanent("reuse_task_input_conflict")
	}
	fingerprint, err := BuildFingerprint(ctx, j.OrganizationID, input.Identity, input.ProviderInput, h.Digests)
	if errors.Is(err, ErrDigestUnavailable) {
		return Output{Hit: false}, nil
	}
	if errors.Is(err, ErrInvalidFingerprintInput) {
		return Output{}, taskerror.Permanent("reuse_generation_input_invalid")
	}
	if err != nil {
		return Output{}, taskerror.Retryable("reuse_input_storage_unavailable")
	}
	miss := Output{Hit: false, Fingerprint: fingerprint}
	if input.Policy == "force_new" {
		return miss, nil
	}
	entries, err := h.Evidence.Find(ctx, boundary, fingerprint)
	if err != nil {
		return Output{}, taskerror.Retryable("reuse_evidence_unavailable")
	}
	for _, entry := range entries {
		if entry.Validate() != nil || entry.Boundary != boundary || entry.Fingerprint != fingerprint || entry.AssetKind != input.Identity.OutputAssetKind {
			return Output{}, taskerror.Permanent("reuse_evidence_invalid")
		}
		resource, err := h.VerifyEvidence(ctx, entry, input.Identity)
		if errors.Is(err, client.ErrReusableAssetNotFound) {
			continue
		}
		if err != nil {
			return Output{}, taskerror.Retryable("reuse_asset_verification_unavailable")
		}
		return Output{Hit: true, Fingerprint: fingerprint, EvidenceID: entry.ID, SourceStepRunID: entry.SourceStepRunID, Resource: resource}, nil
	}
	return miss, nil
}

// VerifyEvidence revalidates the real Asset at both check and finalization.
// Evidence and Step projections never grant access by themselves.
func (h Handler) VerifyEvidence(ctx context.Context, entry domain.ReuseEvidence, identity Identity) (executable.StepResource, error) {
	if h.Assets == nil {
		return executable.StepResource{}, ErrMediaResolverUnavailable
	}
	a, err := h.Assets.VerifyReusableGeneratedAsset(ctx, entry.AssetID, entry.Boundary.OrganizationID, entry.Boundary.Scope, entry.Boundary.OwnerUserID)
	if err != nil {
		return executable.StepResource{}, err
	}
	if a == nil || a.ID != entry.AssetID || a.OrganizationID != entry.Boundary.OrganizationID || a.Scope != entry.Boundary.Scope || a.OwnerUserID != entry.Boundary.OwnerUserID || a.JobID != entry.SourceJobID || a.StepRunID != entry.SourceStepRunID || a.TemplateID != identity.TemplateID || a.TemplateVersion != identity.TemplateVersion || a.AssetKind != entry.AssetKind || a.ObjectPath != entry.ObjectPath || a.Content != entry.Content {
		return executable.StepResource{}, client.ErrReusableAssetNotFound
	}
	result := executable.StepResource{ID: a.ID, Content: a.Content, AssetKind: a.AssetKind}
	if a.ObjectPath == "" {
		return result, nil
	}
	origin, err := url.Parse(h.StorageOrigin)
	bucket := h.StorageBucket
	if bucket == "" {
		bucket = "media"
	}
	if err != nil || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || strings.Trim(origin.Path, "/") != "" || (origin.Scheme != "http" && origin.Scheme != "https") || strings.ContainsAny(bucket, "/\\?#") {
		return executable.StepResource{}, ErrMediaResolverUnavailable
	}
	origin.Path = "/" + bucket + "/" + a.ObjectPath
	result.URL, result.AssetRef = origin.String(), "object_path:"+a.ObjectPath
	return result, nil
}
func equalInput(a, b Input) bool {
	first, e1 := json.Marshal(a)
	second, e2 := json.Marshal(b)
	return e1 == nil && e2 == nil && string(first) == string(second)
}
