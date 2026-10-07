package finalize_task

import (
	"context"
	"encoding/json"
	"net/url"
	"path"
	"strings"

	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/shared/taskcontext"
	"media_agent/media_workflow/biz/shared/taskerror"
	"media_agent/media_workflow/biz/shared/taskref"
)

type JobLookup interface {
	Get(context.Context, string) (*domain.Job, error)
}
type StepLookup interface {
	GetByJobAndStepKey(context.Context, string, string) (*domain.StepRun, error)
}
type ExecutionLookup interface {
	Get(context.Context, string) (*domain.EngineExecution, error)
}
type AssetRegistrar interface {
	UpsertGeneratedAsset(context.Context, *client.UpsertGenAssetReq) (string, error)
}

type Handler struct {
	Jobs          JobLookup
	Steps         StepLookup
	Executions    ExecutionLookup
	Assets        AssetRegistrar
	StorageOrigin string
	StorageBucket string
}

func (h Handler) Execute(ctx context.Context, input Input) (Output, error) {
	identity, ok := taskcontext.FromContext(ctx)
	if !ok || identity.TaskType != TaskDefinitionName || input.JobID == "" || input.StepKey == "" {
		return Output{}, taskerror.Permanent("finalize_identity_invalid")
	}
	if h.Jobs == nil || h.Steps == nil || h.Executions == nil || h.Assets == nil {
		return Output{}, taskerror.Retryable("finalize_dependencies_unavailable")
	}
	j, err := h.Jobs.Get(ctx, input.JobID)
	if err != nil || j == nil {
		return Output{}, taskerror.Retryable("finalize_job_unavailable")
	}
	if j.ConductorWorkflowID == "" {
		return Output{}, taskerror.Retryable("finalize_execution_not_bound")
	}
	if j.ConductorWorkflowID != identity.WorkflowID || j.OrganizationID == "" || j.Scope == "" || j.SubmittedBy == "" || j.Status == domain.JobCancelled {
		return Output{}, taskerror.Permanent("finalize_job_identity_conflict")
	}
	step, err := h.Steps.GetByJobAndStepKey(ctx, j.ID, input.StepKey)
	if err != nil || step == nil {
		return Output{}, taskerror.Retryable("finalize_step_unavailable")
	}
	if step.JobID != j.ID || step.StepKey != input.StepKey || step.TemplateID == "" || step.TemplateVersion <= 0 || step.AssetKind != input.OutputAssetKind {
		return Output{}, taskerror.Permanent("finalize_step_identity_conflict")
	}
	execution, err := h.Executions.Get(ctx, identity.WorkflowID)
	if err != nil || execution == nil {
		return Output{}, taskerror.Retryable("finalize_execution_unavailable")
	}
	if execution.WorkflowInstanceID != identity.WorkflowID || execution.Status != "RUNNING" {
		return Output{}, taskerror.Permanent("finalize_execution_inactive")
	}
	var native *domain.EngineTask
	for i := range execution.Tasks {
		task := &execution.Tasks[i]
		if task.TaskID == identity.TaskID {
			if native != nil {
				return Output{}, taskerror.Permanent("finalize_task_ambiguous")
			}
			native = task
		}
	}
	if native == nil || native.Retried || native.TaskType != TaskDefinitionName || native.ReferenceTaskName != identity.Reference ||
		(native.Status != "IN_PROGRESS" && native.Status != "SCHEDULED") {
		return Output{}, taskerror.Permanent("finalize_task_inactive")
	}
	var frozen Input
	if decode(native.Input, &frozen) != nil || !equalInput(frozen, input) {
		return Output{}, taskerror.Permanent("finalize_input_conflict")
	}
	invocation, err := successfulInvocation(execution, input.StepKey)
	if err != nil {
		return Output{}, err
	}
	var candidates []Candidate
	if decode(invocation.Output["candidates"], &candidates) != nil || !equalCandidates(candidates, input.Candidates) {
		return Output{}, taskerror.Permanent("finalize_candidates_conflict")
	}
	selected, err := selectCandidate(input)
	if err != nil {
		return Output{}, taskerror.Permanent("finalize_candidate_invalid")
	}
	if input.SelectedCandidate != nil {
		if err := verifyApproval(execution, input.StepKey, invocation, selected); err != nil {
			return Output{}, err
		}
	}
	registrationURL, assetRef, err := h.mediaReference(j, selected)
	if err != nil {
		return Output{}, err
	}
	resultKey, err := domain.GenerationResultKey(identity.WorkflowID, step.ID, invocation.TaskID)
	if err != nil {
		return Output{}, taskerror.Permanent("finalize_result_identity_invalid")
	}
	nameID := step.ID
	if len(nameID) > 8 {
		nameID = nameID[:8]
	}
	request := &client.UpsertGenAssetReq{Scope: j.Scope, AssetKind: step.AssetKind, Name: step.StepKey + "_" + nameID,
		URL: registrationURL, Content: selected.Content, OwnerUserID: j.SubmittedBy, Visibility: "private",
		JobID: j.ID, StepRunID: step.ID, TemplateID: step.TemplateID, TemplateVersion: step.TemplateVersion, ResultKey: resultKey}
	if j.Input.Product != nil {
		request.SourceProductID = j.Input.Product.ID
	}
	if j.Input.VirtualIP != nil {
		request.SourceVirtualIPID = j.Input.VirtualIP.ID
	}
	id, err := h.Assets.UpsertGeneratedAsset(ctx, request)
	if err != nil {
		if taskerror.IsPermanent(err) {
			return Output{}, taskerror.Permanent("finalize_registration_rejected")
		}
		return Output{}, taskerror.Retryable("finalize_registration_unavailable")
	}
	if !strings.HasPrefix(id, "gen_") {
		return Output{}, taskerror.Retryable("finalize_registration_identity_invalid")
	}
	return Output{OutputResourceID: id, OutputResource: Resource{ID: id, AssetRef: assetRef, URL: registrationURL, Content: selected.Content, AssetKind: step.AssetKind}}, nil
}

func successfulInvocation(execution *domain.EngineExecution, stepKey string) (*domain.EngineTask, error) {
	latest := 0
	for _, task := range execution.Tasks {
		key, phase, iteration, ok := taskref.Parse(task.ReferenceTaskName)
		if !ok || key != stepKey {
			continue
		}
		if phase == "loop" {
			var round int
			if decode(task.Output["iteration"], &round) == nil && round > iteration {
				iteration = round
			}
		}
		if iteration > latest {
			latest = iteration
		}
	}
	var successful *domain.EngineTask
	for i := range execution.Tasks {
		task := &execution.Tasks[i]
		key, phase, iteration, ok := taskref.Parse(task.ReferenceTaskName)
		// Compatibility with already frozen pre-loop Definitions.
		legacy := task.ReferenceTaskName == "step__"+stepKey+"__invoke" && latest == 0
		if task.Retried || (!legacy && (!ok || key != stepKey || phase != "invoke" || iteration != latest)) {
			continue
		}
		if task.Status == "IN_PROGRESS" || task.Status == "SCHEDULED" {
			return nil, taskerror.Permanent("finalize_generation_pending")
		}
		if task.Status != "COMPLETED" {
			continue
		}
		if successful != nil || task.TaskID == "" {
			return nil, taskerror.Permanent("finalize_generation_ambiguous")
		}
		successful = task
	}
	if successful == nil {
		return nil, taskerror.Permanent("finalize_generation_missing")
	}
	return successful, nil
}

func verifyApproval(execution *domain.EngineExecution, key string, invoke *domain.EngineTask, selected Candidate) error {
	_, _, round, _ := taskref.Parse(invoke.ReferenceTaskName)
	var approved bool
	for _, task := range execution.Tasks {
		step, phase, iteration, ok := taskref.Parse(task.ReferenceTaskName)
		legacy := round == 0 && task.ReferenceTaskName == "step__"+key+"__approval"
		if task.Retried || (!legacy && (!ok || step != key || phase != "approval" || iteration != round)) {
			continue
		}
		if task.TaskType != "HUMAN" || task.Status != "COMPLETED" || task.Output["decision"] != "accept" || approved {
			return taskerror.Permanent("finalize_approval_conflict")
		}
		var candidate Candidate
		if decode(task.Output["selected_candidate"], &candidate) != nil || candidate != selected {
			return taskerror.Permanent("finalize_approval_conflict")
		}
		approved = true
	}
	if !approved {
		return taskerror.Permanent("finalize_approval_missing")
	}
	return nil
}

func decode(raw any, target any) error {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}

func equalInput(a, b Input) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

func equalCandidates(a, b []Candidate) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (h Handler) mediaReference(j *domain.Job, candidate Candidate) (string, string, error) {
	if candidate.URL == "" && candidate.ObjectPath == "" {
		return "", "", nil
	}
	origin, err := url.Parse(h.StorageOrigin)
	if err != nil || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || strings.Trim(origin.Path, "/") != "" {
		return "", "", taskerror.Retryable("finalize_storage_unconfigured")
	}
	objectPath := candidate.ObjectPath
	bucket := h.StorageBucket
	if bucket == "" {
		bucket = "media"
	}
	if strings.ContainsAny(bucket, "/\\?#") {
		return "", "", taskerror.Retryable("finalize_storage_unconfigured")
	}
	if candidate.URL != "" {
		var fromURL string
		if strings.HasPrefix(candidate.URL, "object_path:") {
			fromURL = strings.TrimPrefix(candidate.URL, "object_path:")
		} else {
			parsed, err := url.Parse(candidate.URL)
			if err != nil || parsed.User != nil || parsed.Fragment != "" || parsed.Scheme != origin.Scheme || parsed.Host != origin.Host || !strings.HasPrefix(parsed.Path, "/"+bucket+"/") {
				return "", "", taskerror.Permanent("finalize_media_origin_invalid")
			}
			fromURL = strings.TrimPrefix(parsed.Path, "/"+bucket+"/")
		}
		if objectPath != "" && objectPath != fromURL {
			return "", "", taskerror.Permanent("finalize_media_reference_conflict")
		}
		objectPath = fromURL
	}
	prefix := "materials/" + j.OrganizationID + "/" + j.Scope + "/"
	if !strings.HasPrefix(objectPath, prefix) || path.Clean(objectPath) != objectPath || strings.ContainsAny(objectPath, "\\?#") || strings.ContainsAny(j.OrganizationID+j.Scope, "/\\") {
		return "", "", taskerror.Permanent("finalize_media_namespace_invalid")
	}
	origin.Path = "/" + bucket + "/" + objectPath
	return origin.String(), "object_path:" + objectPath, nil
}
