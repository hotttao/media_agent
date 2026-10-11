package finalize_task

import (
	"context"
	"errors"
	"reflect"
	"strings"

	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/shared/taskerror"
	"media_agent/media_workflow/biz/shared/taskref"
	"media_agent/media_workflow/biz/workflow/reuse_task"
)

// Only the completed native check Task can authorize the hit branch. A caller
// cannot inject an Asset into Finalize by changing its resolved input.
func verifyReuseCheck(execution *domain.EngineExecution, input Input, step *domain.StepRun) error {
	if input.ReuseResult == nil || input.FingerprintIdentity.TemplateID != step.TemplateID || input.FingerprintIdentity.TemplateVersion != step.TemplateVersion || input.FingerprintIdentity.OutputAssetKind != step.AssetKind {
		return taskerror.Permanent("finalize_reuse_contract_invalid")
	}
	var found bool
	for _, task := range execution.Tasks {
		if task.Retried || task.ReferenceTaskName != taskref.Reference(input.StepKey, "reuse") {
			continue
		}
		var request reuse_task.Input
		var result reuse_task.Output
		if found || task.TaskType != reuse_task.TaskDefinitionName || task.Status != "COMPLETED" || decode(task.Input, &request) != nil || decode(task.Output, &result) != nil || request.JobID != input.JobID || request.StepKey != input.StepKey || request.Policy != input.ReusePolicy || !reflect.DeepEqual(request.Identity, *input.FingerprintIdentity) || result != *input.ReuseResult {
			return taskerror.Permanent("finalize_reuse_check_conflict")
		}
		found = true
	}
	if !found {
		return taskerror.Permanent("finalize_reuse_check_missing")
	}
	return nil
}

func reuseBoundary(j *domain.Job) domain.ReuseScope {
	return domain.ReuseScope{OrganizationID: j.OrganizationID, WorkspaceID: j.WorkspaceID, OwnerUserID: j.SubmittedBy, Scope: j.Scope}
}

func (h Handler) finalizeReuse(ctx context.Context, j *domain.Job, input Input) (Output, error) {
	if h.Evidence == nil || h.ReusableAssets == nil {
		return Output{}, taskerror.Retryable("finalize_reuse_dependencies_unavailable")
	}
	boundary := reuseBoundary(j)
	if j.Origin != "native" || !boundary.Valid() || input.ReusePolicy != "reuse_if_available" {
		return Output{}, taskerror.Permanent("finalize_reuse_boundary_invalid")
	}
	entries, err := h.Evidence.Find(ctx, boundary, input.ReuseResult.Fingerprint)
	if err != nil {
		return Output{}, taskerror.Retryable("finalize_reuse_evidence_unavailable")
	}
	for _, entry := range entries {
		if entry.ID != input.ReuseResult.EvidenceID {
			continue
		}
		if entry.Validate() != nil || entry.Boundary != boundary || entry.Fingerprint != input.ReuseResult.Fingerprint || entry.SourceStepRunID != input.ReuseResult.SourceStepRunID || entry.AssetKind != input.OutputAssetKind {
			return Output{}, taskerror.Permanent("finalize_reuse_evidence_invalid")
		}
		verifier := reuse_task.Handler{Assets: h.ReusableAssets, StorageOrigin: h.StorageOrigin, StorageBucket: h.StorageBucket}
		resource, err := verifier.VerifyEvidence(ctx, entry, *input.FingerprintIdentity)
		if errors.Is(err, client.ErrReusableAssetNotFound) {
			// Do not locally replan a branch already chosen by Conductor.
			return Output{}, taskerror.Permanent("finalize_reuse_source_gone")
		}
		if err != nil {
			return Output{}, taskerror.Retryable("finalize_reuse_verification_unavailable")
		}
		if resource != input.ReuseResult.Resource {
			return Output{}, taskerror.Permanent("finalize_reuse_resource_conflict")
		}
		return Output{OutputResourceID: resource.ID, OutputResource: resource, ResultSource: "reused", ReusedFromStepRunID: entry.SourceStepRunID, ReuseEvidenceID: entry.ID}, nil
	}
	return Output{}, taskerror.Permanent("finalize_reuse_source_gone")
}

func (h Handler) recordGeneration(ctx context.Context, j *domain.Job, step *domain.StepRun, invocation *domain.EngineTask, input Input, output Output, resultKey string) error {
	if h.Evidence == nil {
		return taskerror.Retryable("finalize_reuse_dependencies_unavailable")
	}
	if j.Origin != "native" || !reuseBoundary(j).Valid() {
		return taskerror.Permanent("finalize_reuse_boundary_invalid")
	}
	// A regeneration may have feedback in its final prompt. Never record the
	// initial check's fingerprint against a differently generated result.
	fingerprint, err := reuse_task.BuildFingerprint(ctx, j.OrganizationID, *input.FingerprintIdentity, invocation.Input, h.Digests)
	if errors.Is(err, reuse_task.ErrDigestUnavailable) {
		return nil // Legacy objects without content SHA are deliberately not indexed.
	}
	if errors.Is(err, reuse_task.ErrInvalidFingerprintInput) {
		return taskerror.Permanent("finalize_generation_fingerprint_invalid")
	}
	if err != nil {
		return taskerror.Retryable("finalize_generation_digest_unavailable")
	}
	evidence := domain.ReuseEvidence{ID: domain.ReuseEvidenceID(step.ID, resultKey), Boundary: reuseBoundary(j), Fingerprint: fingerprint, SourceJobID: j.ID, SourceStepRunID: step.ID, SourceResultKey: resultKey, AssetID: output.OutputResourceID, ObjectPath: strings.TrimPrefix(output.OutputResource.AssetRef, "object_path:"), Content: output.OutputResource.Content, AssetKind: step.AssetKind}
	if err := h.Evidence.Save(ctx, evidence); err != nil {
		if errors.Is(err, domain.ErrReuseEvidenceConflict) {
			return taskerror.Permanent("finalize_generation_evidence_conflict")
		}
		return taskerror.Retryable("finalize_generation_evidence_unavailable")
	}
	return nil
}
