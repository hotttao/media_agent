package worker_gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	capabilitydomain "media_agent/media_workflow/biz/domain/capability"
	jobdomain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
)

const defaultLeaseDuration = 90 * time.Second

type Principal struct {
	UserID, ActorID, OrganizationID, CredentialID, AuthSource string
}

type PollCommand struct {
	Principal
	WorkerInstanceID  string
	CapabilityKey     string
	CapabilityVersion int
}

type CapabilityVersion struct {
	Key         string
	Version     int
	Concurrency int
}

type CompleteCommand struct {
	Principal
	DispatchID, WorkerInstanceID, IdempotencyKey string
	Attempt                                      int
	Output                                       map[string]any
}

type FailCommand struct {
	Principal
	DispatchID, WorkerInstanceID, IdempotencyKey, Reason string
	Attempt                                              int
}

type Flow struct {
	StorageOrigin string
	Dispatches    jobdomain.DispatchRepo
	Jobs          jobdomain.JobRepo
	StepRuns      jobdomain.StepRunRepo
	Capabilities  capabilitydomain.Repo
	Engine        jobdomain.CapabilityTaskEngine
	Executions    interface {
		Get(context.Context, string) (*jobdomain.EngineExecution, error)
	}
	ImportedAssets interface {
		GetImportedGeneratedAsset(context.Context, string) (*client.ImportedGeneratedAsset, error)
		ResolveInternalObject(context.Context, string, string) (client.ResolvedStorageObject, error)
	}
	Now   func() time.Time
	Lease time.Duration
}

func (f *Flow) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f *Flow) lease() time.Duration {
	if f.Lease > 0 {
		return f.Lease
	}
	return defaultLeaseDuration
}

func (f *Flow) Register(ctx context.Context, principal Principal, workerInstanceID string, capabilities []CapabilityVersion) error {
	if err := validatePrincipal(principal); err != nil {
		return err
	}
	if workerInstanceID == "" || len(capabilities) == 0 {
		return ErrInvalidRequest
	}
	for _, item := range capabilities {
		if item.Key == "" || item.Version <= 0 || item.Concurrency <= 0 {
			return ErrInvalidRequest
		}
		capability, err := f.Capabilities.Get(ctx, item.Key, item.Version)
		if err != nil {
			return fmt.Errorf("load capability %s@%d: %w", item.Key, item.Version, err)
		}
		if capability.ExecutionTarget != capabilitydomain.ExecutionTargetPersonalWorker {
			return ErrForbidden
		}
	}
	return nil
}

func (f *Flow) Poll(ctx context.Context, cmd PollCommand) (*jobdomain.WorkerDispatch, error) {
	if err := validatePrincipal(cmd.Principal); err != nil {
		return nil, err
	}
	if cmd.WorkerInstanceID == "" || cmd.CapabilityKey == "" || cmd.CapabilityVersion <= 0 {
		return nil, ErrInvalidRequest
	}
	now := f.now()
	if existing, err := f.Dispatches.FindActive(ctx, cmd.UserID, cmd.OrganizationID, cmd.CredentialID, cmd.WorkerInstanceID, cmd.CapabilityKey, cmd.CapabilityVersion, now); err == nil {
		job, err := f.Jobs.Get(ctx, existing.JobID)
		if err != nil {
			return nil, err
		}
		if job.OrganizationID == "" || job.OrganizationID != cmd.OrganizationID || job.SubmittedBy != cmd.UserID || job.ConductorWorkflowID != existing.ConductorWorkflowID {
			return nil, ErrForbidden
		}
		if err := f.requireActiveTask(ctx, job, existing.ConductorWorkflowID, existing.ConductorTaskID, existing.Attempt); err != nil {
			return nil, err
		}
		var input map[string]any
		if err := json.Unmarshal(existing.InputJSON, &input); err != nil {
			return nil, err
		}
		contract, err := f.Capabilities.Get(ctx, existing.CapabilityKey, existing.CapabilityVersion)
		if err != nil {
			return nil, err
		}
		if contract.ExecutionTarget != capabilitydomain.ExecutionTargetPersonalWorker {
			return nil, ErrForbidden
		}
		if err := validateProviderInputs(contract, input); err != nil {
			return nil, err
		}
		resolved, err := f.resolveCapabilityInputRefs(ctx, job, input, contract)
		if err != nil {
			return nil, fmt.Errorf("resolve task inputs: %w", err)
		}
		return dispatchForDelivery(existing, resolved)
	} else if !errors.Is(err, jobdomain.ErrDispatchNotFound) {
		return nil, err
	}
	capability, err := f.Capabilities.Get(ctx, cmd.CapabilityKey, cmd.CapabilityVersion)
	if err != nil {
		return nil, fmt.Errorf("load capability: %w", err)
	}
	if capability.ExecutionTarget != capabilitydomain.ExecutionTargetPersonalWorker {
		return nil, ErrForbidden
	}
	domain := jobdomain.ExecutionDomainForUser(cmd.UserID)
	task, err := f.Engine.PollCapabilityTask(ctx, capability.ConductorTaskDef, "media-workflow-proxy", domain)
	if err != nil || task == nil {
		return nil, err
	}
	jobID, _ := task.Input["job_id"].(string)
	stepKey, _ := task.Input["step_key"].(string)
	if jobID == "" || stepKey == "" {
		return nil, fmt.Errorf("%w: conductor task is missing job_id or step_key", ErrInvalidTask)
	}
	job, err := f.Jobs.Get(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("load task job: %w", err)
	}
	if job.OrganizationID == "" || job.OrganizationID != cmd.OrganizationID ||
		job.SubmittedBy != cmd.UserID || job.ConductorWorkflowID != task.WorkflowInstanceID {
		return nil, ErrForbidden
	}
	stepRun, err := f.StepRuns.GetByJobAndStepKey(ctx, jobID, stepKey)
	if err != nil {
		return nil, fmt.Errorf("load task step run: %w", err)
	}
	if err := validateProviderInputs(capability, task.Input); err != nil {
		return nil, err
	}
	if stepRun.JobID != job.ID || stepRun.StepKey != stepKey || task.TaskType != capability.ConductorTaskDef {
		return nil, ErrForbidden
	}
	if err := f.requireActiveTask(ctx, job, task.WorkflowInstanceID, task.TaskID, task.RetryCount+1); err != nil {
		return nil, err
	}
	resolved, err := f.resolveCapabilityInputRefs(ctx, job, task.Input, capability)
	if err != nil {
		return nil, fmt.Errorf("resolve task inputs: %w", err)
	}
	inputJSON, err := json.Marshal(task.Input)
	if err != nil {
		return nil, err
	}
	attempt := task.RetryCount + 1
	if attempt < 1 {
		attempt = 1
	}
	deliveredAt := now
	dispatch, err := f.Dispatches.Create(ctx, &jobdomain.WorkerDispatch{
		ID: "dispatch_" + uuid.NewString(), ConductorTaskID: task.TaskID, ConductorWorkflowID: task.WorkflowInstanceID,
		JobID: jobID, StepRunID: stepRun.ID, Attempt: attempt,
		CapabilityKey: capability.Key, CapabilityVersion: capability.Version,
		UserID: job.SubmittedBy, OrganizationID: job.OrganizationID, CredentialID: cmd.CredentialID,
		ActorID: cmd.ActorID, AuthSource: cmd.AuthSource,
		WorkerInstanceID: cmd.WorkerInstanceID, Status: jobdomain.DispatchDelivered,
		InputJSON: inputJSON, LeaseExpiresAt: now.Add(f.lease()), DeliveredAt: &deliveredAt,
	})
	if err != nil {
		return nil, err
	}
	return dispatchForDelivery(dispatch, resolved)
}

// Copy the delivery envelope: transport credentials must not replace the
// stable references held by the repository or Conductor task snapshot.
func dispatchForDelivery(dispatch *jobdomain.WorkerDispatch, input map[string]any) (*jobdomain.WorkerDispatch, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	delivery := *dispatch
	delivery.InputJSON = encoded
	return &delivery, nil
}

func (f *Flow) resolveInputRefs(ctx context.Context, job *jobdomain.Job, input map[string]any) (map[string]any, error) {
	return f.resolveTypedInputRefs(ctx, job, input, nil)
}

func (f *Flow) resolveCapabilityInputRefs(ctx context.Context, job *jobdomain.Job, input map[string]any, contract *capabilitydomain.Capability) (map[string]any, error) {
	if err := validateProviderInputs(contract, input); err != nil {
		return nil, err
	}
	expected := make(map[string]capabilitydomain.MediaType)
	if contract.OutputMediaType != "" {
		for _, field := range contract.Inputs {
			if field.MediaType == "" {
				continue
			}
			var refs []string
			switch value := input[field.Name].(type) {
			case string:
				refs = []string{value}
			case []string:
				refs = value
			case []any:
				for _, item := range value {
					refs = append(refs, item.(string))
				}
			}
			for _, ref := range refs {
				if old, exists := expected[ref]; exists && old != field.MediaType {
					return nil, ErrInvalidTask
				}
				expected[ref] = field.MediaType
			}
		}
	}
	return f.resolveTypedInputRefs(ctx, job, input, expected)
}

func (f *Flow) resolveTypedInputRefs(ctx context.Context, job *jobdomain.Job, input map[string]any, expected map[string]capabilitydomain.MediaType) (map[string]any, error) {
	cache := map[string]string{}
	var resolve func(any) (any, error)
	resolve = func(value any) (any, error) {
		switch item := value.(type) {
		case string:
			reference := item
			// An upstream result can carry an old storage signature. Reissue
			// only URLs from the configured storage origin, never arbitrary URLs.
			if f.StorageOrigin != "" {
				base, baseErr := url.Parse(f.StorageOrigin)
				u, urlErr := url.Parse(item)
				if baseErr == nil && urlErr == nil && u.Scheme == base.Scheme && u.Host == base.Host && u.Host != "" {
					if u.User != nil || u.Fragment != "" || !strings.HasPrefix(u.Path, "/media/materials/"+job.OrganizationID+"/") || path.Clean(u.Path) != u.Path || strings.Contains(u.Path, "\\") {
						return nil, ErrForbidden
					}
					reference = "object_path:" + strings.TrimPrefix(u.Path, "/media/")
				}
			}
			if strings.HasPrefix(reference, "object_path:") {
				if f.ImportedAssets == nil {
					return nil, errors.New("execution storage resolver is not configured")
				}
				objectPath := strings.TrimPrefix(reference, "object_path:")
				if !strings.HasPrefix(objectPath, "materials/"+job.OrganizationID+"/") || path.Clean(objectPath) != objectPath || strings.Contains(objectPath, "\\") {
					return nil, ErrForbidden
				}
				if cached := cache[item]; cached != "" {
					return cached, nil
				}
				resolved, err := f.ImportedAssets.ResolveInternalObject(ctx, job.OrganizationID, objectPath)
				if err != nil {
					return nil, err
				}
				if resolved.URL == "" || resolved.ExpiresAt <= f.now().Add(30*time.Second).Unix() {
					return nil, ErrForbidden
				}
				if media := expected[item]; media != "" {
					contentType, _, err := mime.ParseMediaType(resolved.ContentType)
					if err != nil || resolved.SizeBytes <= 0 || !strings.HasPrefix(contentType, strings.ToLower(string(media))+"/") {
						return nil, ErrInvalidTask
					}
				}
				cache[item] = resolved.URL
				return resolved.URL, nil
			}
			if expected[item] != "" {
				return nil, ErrForbidden
			}
			if job.Origin != "legacy_import" || !strings.HasPrefix(item, "gen_") {
				return item, nil
			}
			if f.ImportedAssets == nil {
				return nil, errors.New("historical asset resolver is not configured")
			}
			if cached := cache[item]; cached != "" {
				return cached, nil
			}
			asset, err := f.ImportedAssets.GetImportedGeneratedAsset(ctx, item)
			if err != nil {
				return nil, err
			}
			if asset.ID != item || asset.OrganizationID != job.OrganizationID || asset.OwnerUserID != job.SubmittedBy ||
				asset.Scope != job.Scope || asset.SourceSystem != "media_ai" || asset.AccessURL == "" ||
				asset.AccessExpiresAt <= f.now().Add(30*time.Second).Unix() {
				return nil, ErrForbidden
			}
			cache[item] = asset.AccessURL
			return asset.AccessURL, nil
		case []any:
			out := make([]any, len(item))
			for i, child := range item {
				result, err := resolve(child)
				if err != nil {
					return nil, err
				}
				out[i] = result
			}
			return out, nil
		case []string:
			out := make([]string, len(item))
			for i, child := range item {
				resolved, err := resolve(child)
				if err != nil {
					return nil, err
				}
				out[i] = resolved.(string)
			}
			return out, nil
		case map[string]any:
			out := make(map[string]any, len(item))
			for key, child := range item {
				if key == "prompt" {
					if _, ok := child.(string); ok {
						out[key] = child
						continue
					}
				}
				result, err := resolve(child)
				if err != nil {
					return nil, err
				}
				out[key] = result
			}
			return out, nil
		default:
			return value, nil
		}
	}
	result, err := resolve(input)
	if err != nil {
		return nil, err
	}
	out := result.(map[string]any)
	// Pure template rendering may embed a frozen .url inside the prompt.
	// Substitute only known media references, not arbitrary URLs or scripts.
	if prompt, ok := input["prompt"].(string); ok {
		var refs []string
		if job.Input.Product != nil {
			refs = append(refs, job.Input.Product.ImageURL)
		}
		if vip := job.Input.VirtualIP; vip != nil {
			refs = append(refs, vip.AvatarURL, vip.FullBodyURL, vip.ThreeViewURL, vip.NineViewURL)
		}
		for _, material := range job.Input.Materials {
			refs = append(refs, material.URL)
		}
		for _, asset := range job.Input.Assets {
			refs = append(refs, asset.URL)
		}
		for _, ref := range refs {
			if ref == "" || !strings.Contains(prompt, ref) {
				continue
			}
			if _, err := resolve(ref); err != nil {
				return nil, err
			}
		}
		keys := make([]string, 0, len(cache))
		for ref := range cache {
			keys = append(keys, ref)
		}
		// Longest first prevents a shorter media path replacing a prefix of
		// another input's reference. Lexical tie-break keeps it deterministic.
		sort.Slice(keys, func(i, j int) bool {
			if len(keys[i]) != len(keys[j]) {
				return len(keys[i]) > len(keys[j])
			}
			return keys[i] < keys[j]
		})
		for _, ref := range keys {
			prompt = strings.ReplaceAll(prompt, ref, cache[ref])
		}
		out["prompt"] = prompt
	}
	return out, nil
}

func (f *Flow) Heartbeat(ctx context.Context, principal Principal, dispatchID, workerInstanceID string, attempt int) (*jobdomain.WorkerDispatch, error) {
	dispatch, err := f.authorizeDispatch(ctx, principal, dispatchID, workerInstanceID, attempt)
	if err != nil {
		return nil, err
	}
	callbackSeconds := int(f.lease().Seconds())
	if err := f.Engine.UpdateCapabilityTask(ctx, jobdomain.CapabilityTaskResult{
		WorkflowInstanceID: dispatch.ConductorWorkflowID, TaskID: dispatch.ConductorTaskID,
		WorkerID: "media-workflow-proxy", Status: "IN_PROGRESS", CallbackAfterSeconds: callbackSeconds,
	}); err != nil {
		return nil, err
	}
	return f.Dispatches.ExtendLease(ctx, dispatchID, attempt, f.now().Add(f.lease()))
}

func (f *Flow) Complete(ctx context.Context, cmd CompleteCommand) (*jobdomain.WorkerResultReceipt, error) {
	if len(cmd.Output) == 0 {
		return nil, ErrInvalidOutput
	}
	if !validCandidates(cmd.Output["candidates"]) {
		return nil, fmt.Errorf("%w: at least one valid candidate is required", ErrInvalidOutput)
	}
	return f.saveResult(ctx, cmd.Principal, cmd.DispatchID, cmd.WorkerInstanceID, cmd.Attempt, "complete", cmd.IdempotencyKey, cmd.Output)
}

func validCandidates(raw any) bool {
	var candidates []any
	switch value := raw.(type) {
	case []any:
		candidates = value
	case []map[string]any:
		candidates = make([]any, len(value))
		for i := range value {
			candidates[i] = value[i]
		}
	default:
		return false
	}
	if len(candidates) == 0 {
		return false
	}
	for _, rawCandidate := range candidates {
		candidate, ok := rawCandidate.(map[string]any)
		if !ok {
			return false
		}
		url, _ := candidate["url"].(string)
		content, _ := candidate["content"].(string)
		objectPath, _ := candidate["object_path"].(string)
		if url == "" && content == "" && objectPath == "" {
			return false
		}
	}
	return true
}

func (f *Flow) Fail(ctx context.Context, cmd FailCommand) (*jobdomain.WorkerResultReceipt, error) {
	if cmd.Reason == "" {
		return nil, ErrInvalidRequest
	}
	return f.saveResult(ctx, cmd.Principal, cmd.DispatchID, cmd.WorkerInstanceID, cmd.Attempt, "fail", cmd.IdempotencyKey, map[string]any{"reason": cmd.Reason})
}

func (f *Flow) saveResult(ctx context.Context, principal Principal, dispatchID, workerInstanceID string, attempt int, operation, idempotencyKey string, result map[string]any) (*jobdomain.WorkerResultReceipt, error) {
	if idempotencyKey == "" {
		return nil, ErrInvalidRequest
	}
	if _, err := f.authorizeDispatch(ctx, principal, dispatchID, workerInstanceID, attempt); err != nil {
		return nil, err
	}
	return f.persistResult(ctx, dispatchID, attempt, operation, idempotencyKey, result)
}

func (f *Flow) persistResult(ctx context.Context, dispatchID string, attempt int, operation, idempotencyKey string, result map[string]any) (*jobdomain.WorkerResultReceipt, error) {
	if idempotencyKey == "" {
		return nil, ErrInvalidRequest
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, ErrInvalidOutput
	}
	digest := sha256.Sum256(encoded)
	receipt, err := f.Dispatches.SaveResult(ctx, dispatchID, attempt, operation, idempotencyKey, encoded, hex.EncodeToString(digest[:]), f.now())
	if err != nil {
		return nil, err
	}
	if receipt.ConductorStatus == "submitted" {
		return receipt, nil
	}
	// The durable submitter owns Conductor delivery. HTTP completion only
	// acknowledges that the result is safely stored in the inbox.
	return receipt, nil
}

func (f *Flow) authorizeDispatch(ctx context.Context, principal Principal, dispatchID, workerInstanceID string, attempt int) (*jobdomain.WorkerDispatch, error) {
	if err := validatePrincipal(principal); err != nil {
		return nil, err
	}
	dispatch, err := f.Dispatches.Get(ctx, dispatchID)
	if err != nil {
		return nil, err
	}
	if dispatch.UserID != principal.UserID || dispatch.OrganizationID != principal.OrganizationID ||
		dispatch.CredentialID != principal.CredentialID || dispatch.ActorID != principal.ActorID ||
		dispatch.AuthSource != principal.AuthSource || dispatch.WorkerInstanceID != workerInstanceID {
		return nil, ErrForbidden
	}
	return f.checkDispatchLease(ctx, dispatch, attempt)
}

func (f *Flow) checkDispatchLease(ctx context.Context, dispatch *jobdomain.WorkerDispatch, attempt int) (*jobdomain.WorkerDispatch, error) {
	if dispatch.Attempt != attempt {
		return nil, jobdomain.ErrDispatchConflict
	}
	if !dispatch.LeaseActive(f.now()) && dispatch.Status != jobdomain.DispatchResultReceived {
		return nil, jobdomain.ErrDispatchExpired
	}
	if dispatch.Status != jobdomain.DispatchResultReceived {
		job, err := f.Jobs.Get(ctx, dispatch.JobID)
		if err != nil {
			return nil, fmt.Errorf("load dispatch job: %w", err)
		}
		if err := f.requireActiveTask(ctx, job, dispatch.ConductorWorkflowID, dispatch.ConductorTaskID, dispatch.Attempt); err != nil {
			return nil, err
		}
	}
	return dispatch, nil
}

func (f *Flow) SubmitReceipt(ctx context.Context, receipt *jobdomain.WorkerResultReceipt) error {
	if receipt == nil || receipt.Dispatch == nil {
		return ErrInvalidRequest
	}
	var output map[string]any
	if err := json.Unmarshal(receipt.ResultJSON, &output); err != nil {
		return err
	}
	status := "COMPLETED"
	reason := ""
	if receipt.Operation == "fail" {
		status = "FAILED_WITH_TERMINAL_ERROR"
		reason, _ = output["reason"].(string)
		output = nil
	}
	err := f.Engine.UpdateCapabilityTask(ctx, jobdomain.CapabilityTaskResult{
		WorkflowInstanceID: receipt.Dispatch.ConductorWorkflowID,
		TaskID:             receipt.Dispatch.ConductorTaskID, WorkerID: "media-workflow-proxy",
		Status: status, Output: output, Reason: reason,
	})
	if err != nil {
		_ = f.Dispatches.MarkResultFailed(ctx, receipt.ID, err.Error())
		return err
	}
	return f.Dispatches.MarkResultSubmitted(ctx, receipt.ID, f.now())
}

func (f *Flow) RunResultSubmitter(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			receipts, err := f.Dispatches.ListPendingResults(ctx, 20)
			if err != nil {
				continue
			}
			for _, receipt := range receipts {
				_ = f.SubmitReceipt(ctx, receipt)
			}
		}
	}
}

func (f *Flow) RunLeaseReaper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = f.Dispatches.ExpireLeases(ctx, f.now())
		}
	}
}

func validatePrincipal(principal Principal) error {
	if principal.UserID == "" || !strings.HasPrefix(principal.ActorID, "Worker:") ||
		principal.OrganizationID == "" || principal.CredentialID == "" || principal.AuthSource != "api_token" {
		return ErrUnauthorized
	}
	return nil
}

var (
	ErrUnauthorized   = errors.New("worker principal is incomplete")
	ErrForbidden      = errors.New("worker is not allowed to access this dispatch")
	ErrInvalidRequest = errors.New("invalid worker request")
	ErrInvalidTask    = errors.New("invalid conductor capability task")
	ErrInvalidOutput  = errors.New("invalid capability output")
)
