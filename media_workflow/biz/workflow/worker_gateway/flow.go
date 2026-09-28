package worker_gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	capabilitydomain "media_agent/media_workflow/biz/domain/capability"
	jobdomain "media_agent/media_workflow/biz/domain/job"
)

const defaultLeaseDuration = 90 * time.Second

type Principal struct {
	UserID, OrganizationID, CredentialID string
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
	Dispatches   jobdomain.DispatchRepo
	Jobs         jobdomain.JobRepo
	StepRuns     jobdomain.StepRunRepo
	Capabilities capabilitydomain.Repo
	Engine       jobdomain.CapabilityTaskEngine
	Now          func() time.Time
	Lease        time.Duration
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
		return existing, nil
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
	if job.SubmittedBy != cmd.UserID || job.ConductorWorkflowID != task.WorkflowInstanceID || job.Status.IsTerminal() {
		return nil, ErrForbidden
	}
	stepRun, err := f.StepRuns.GetByJobAndStepKey(ctx, jobID, stepKey)
	if err != nil {
		return nil, fmt.Errorf("load task step run: %w", err)
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
	return f.Dispatches.Create(ctx, &jobdomain.WorkerDispatch{
		ID: "dispatch_" + uuid.NewString(), ConductorTaskID: task.TaskID, ConductorWorkflowID: task.WorkflowInstanceID,
		JobID: jobID, StepRunID: stepRun.ID, Attempt: attempt,
		CapabilityKey: capability.Key, CapabilityVersion: capability.Version,
		UserID: cmd.UserID, OrganizationID: cmd.OrganizationID, CredentialID: cmd.CredentialID,
		WorkerInstanceID: cmd.WorkerInstanceID, Status: jobdomain.DispatchDelivered,
		InputJSON: inputJSON, LeaseExpiresAt: now.Add(f.lease()), DeliveredAt: &deliveredAt,
	})
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
		dispatch.CredentialID != principal.CredentialID || dispatch.WorkerInstanceID != workerInstanceID {
		return nil, ErrForbidden
	}
	if dispatch.Attempt != attempt {
		return nil, jobdomain.ErrDispatchConflict
	}
	if dispatch.Status != jobdomain.DispatchResultReceived {
		job, err := f.Jobs.Get(ctx, dispatch.JobID)
		if err != nil {
			return nil, fmt.Errorf("load dispatch job: %w", err)
		}
		if job.Status.IsTerminal() {
			return nil, jobdomain.ErrDispatchExpired
		}
	}
	if !dispatch.LeaseActive(f.now()) && dispatch.Status != jobdomain.DispatchResultReceived {
		return nil, jobdomain.ErrDispatchExpired
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
	if principal.UserID == "" || principal.OrganizationID == "" || principal.CredentialID == "" {
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
