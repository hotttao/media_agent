package worker_gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	capabilitydomain "media_agent/media_workflow/biz/domain/capability"
	jobdomain "media_agent/media_workflow/biz/domain/job"
)

// ManagedBinding is supplied only by server configuration, never by Worker API.
type ManagedBinding struct {
	CapabilityKey     string
	CapabilityVersion int
	PoolKey           string
}

type ManagedRuntime struct {
	flow                *Flow
	actorID, instanceID string
	bindings            map[string]ManagedBinding
}

func managedKey(key string, version int) string { return fmt.Sprintf("%s@%d", key, version) }

func NewManagedRuntime(ctx context.Context, flow *Flow, workerID, instanceID string, bindings []ManagedBinding) (*ManagedRuntime, error) {
	if flow == nil || strings.TrimSpace(workerID) == "" || strings.Contains(workerID, ":") || strings.TrimSpace(instanceID) == "" || len(bindings) == 0 {
		return nil, ErrInvalidRequest
	}
	r := &ManagedRuntime{flow: flow, actorID: "Worker:" + workerID, instanceID: instanceID, bindings: map[string]ManagedBinding{}}
	routes := map[string]bool{}
	for _, binding := range bindings {
		if binding.CapabilityKey == "" || binding.CapabilityVersion <= 0 || binding.PoolKey == "" {
			return nil, ErrInvalidRequest
		}
		key := managedKey(binding.CapabilityKey, binding.CapabilityVersion)
		if _, exists := r.bindings[key]; exists {
			return nil, ErrInvalidRequest
		}
		capability, err := r.capability(ctx, binding)
		if err != nil {
			return nil, err
		}
		// Conductor queues are keyed by task definition and domain, not by
		// capability version. Two consumers on the same route would steal
		// each other's tasks before frozen-version validation can run.
		route := capability.ConductorTaskDef + ":" + jobdomain.ExecutionDomainForManagedPool(binding.PoolKey)
		if routes[route] {
			return nil, ErrInvalidRequest
		}
		routes[route] = true
		r.bindings[key] = binding
	}
	return r, nil
}

func (r *ManagedRuntime) capability(ctx context.Context, binding ManagedBinding) (*capabilitydomain.Capability, error) {
	capability, err := r.flow.Capabilities.Get(ctx, binding.CapabilityKey, binding.CapabilityVersion)
	if err != nil {
		return nil, err
	}
	if capability.Status != "published" || capability.ExecutionTarget != capabilitydomain.ExecutionTargetManagedWorker || capability.ManagedPoolKey != binding.PoolKey {
		return nil, ErrForbidden
	}
	return capability, nil
}

func (r *ManagedRuntime) Poll(ctx context.Context, key string, version int) (*jobdomain.WorkerDispatch, error) {
	binding, allowed := r.bindings[managedKey(key, version)]
	if !allowed {
		return nil, ErrForbidden
	}
	capability, err := r.capability(ctx, binding)
	if err != nil {
		return nil, err
	}
	now := r.flow.now()
	existing, err := r.flow.Dispatches.FindActiveManaged(ctx, r.actorID, r.instanceID, key, version, now)
	if err == nil {
		job, err := r.flow.Jobs.Get(ctx, existing.JobID)
		if err != nil {
			return nil, err
		}
		if job.OrganizationID == "" || job.SubmittedBy != existing.UserID || job.OrganizationID != existing.OrganizationID || job.ConductorWorkflowID != existing.ConductorWorkflowID {
			return nil, ErrForbidden
		}
		if err := r.flow.requireActiveTask(ctx, job, existing.ConductorWorkflowID, existing.ConductorTaskID, existing.Attempt); err != nil {
			return nil, err
		}
		var input map[string]any
		if err := json.Unmarshal(existing.InputJSON, &input); err != nil {
			return nil, err
		}
		if err := validateProviderInputs(capability, input); err != nil {
			return nil, err
		}
		resolved, err := r.flow.resolveCapabilityInputRefs(ctx, job, input, capability)
		if err != nil {
			return nil, err
		}
		return r.flow.dispatchForDelivery(ctx, existing, resolved)
	}
	if !errors.Is(err, jobdomain.ErrDispatchNotFound) {
		return nil, err
	}
	task, err := r.flow.Engine.PollCapabilityTask(ctx, capability.ConductorTaskDef, r.instanceID, jobdomain.ExecutionDomainForManagedPool(binding.PoolKey))
	if err != nil || task == nil {
		return nil, err
	}
	if task.Input["capability_key"] != key || fmt.Sprint(task.Input["capability_version"]) != fmt.Sprint(version) {
		return nil, ErrInvalidTask
	}
	jobID, _ := task.Input["job_id"].(string)
	stepKey, _ := task.Input["step_key"].(string)
	if jobID == "" || stepKey == "" {
		return nil, ErrInvalidTask
	}
	job, err := r.flow.Jobs.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.OrganizationID == "" || job.SubmittedBy == "" || job.ConductorWorkflowID != task.WorkflowInstanceID {
		return nil, ErrForbidden
	}
	step, err := r.flow.StepRuns.GetByJobAndStepKey(ctx, jobID, stepKey)
	if err != nil {
		return nil, err
	}
	if err := validateProviderInputs(capability, task.Input); err != nil {
		return nil, err
	}
	if step.JobID != job.ID || step.StepKey != stepKey || task.TaskType != capability.ConductorTaskDef {
		return nil, ErrForbidden
	}
	if err := r.flow.requireActiveTask(ctx, job, task.WorkflowInstanceID, task.TaskID, task.RetryCount+1); err != nil {
		return nil, err
	}
	input, err := json.Marshal(task.Input)
	if err != nil {
		return nil, err
	}
	resolved, err := r.flow.resolveCapabilityInputRefs(ctx, job, task.Input, capability)
	if err != nil {
		return nil, err
	}
	deliveredAt := now
	attempt := task.RetryCount + 1
	if attempt < 1 {
		attempt = 1
	}
	dispatch, err := r.flow.Dispatches.Create(ctx, &jobdomain.WorkerDispatch{
		ID: "dispatch_" + uuid.NewString(), ConductorTaskID: task.TaskID, ConductorWorkflowID: task.WorkflowInstanceID,
		JobID: jobID, StepRunID: step.ID, Attempt: attempt, CapabilityKey: key, CapabilityVersion: version,
		UserID: job.SubmittedBy, OrganizationID: job.OrganizationID, ActorID: r.actorID, AuthSource: "internal_runtime",
		WorkerInstanceID: r.instanceID, Status: jobdomain.DispatchDelivered, InputJSON: input,
		LeaseExpiresAt: now.Add(r.flow.lease()), DeliveredAt: &deliveredAt,
	})
	if err != nil {
		return nil, err
	}
	return r.flow.dispatchForDelivery(ctx, dispatch, resolved)
}

func (r *ManagedRuntime) authorize(ctx context.Context, id string, attempt int) (*jobdomain.WorkerDispatch, error) {
	dispatch, err := r.authorizeResultIdentity(ctx, id, attempt)
	if err != nil {
		return nil, err
	}
	return r.flow.requireActiveDispatch(ctx, dispatch, attempt)
}

func (r *ManagedRuntime) authorizeResultIdentity(ctx context.Context, id string, attempt int) (*jobdomain.WorkerDispatch, error) {
	dispatch, err := r.flow.Dispatches.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if dispatch.ActorID != r.actorID || dispatch.WorkerInstanceID != r.instanceID || dispatch.CredentialID != "" || dispatch.AuthSource != "internal_runtime" {
		return nil, ErrForbidden
	}
	binding, allowed := r.bindings[managedKey(dispatch.CapabilityKey, dispatch.CapabilityVersion)]
	if !allowed {
		return nil, ErrForbidden
	}
	if _, err := r.capability(ctx, binding); err != nil {
		return nil, err
	}
	if dispatch.Attempt != attempt {
		return nil, jobdomain.ErrDispatchConflict
	}
	return dispatch, nil
}

func (r *ManagedRuntime) Heartbeat(ctx context.Context, id string, attempt int) error {
	dispatch, err := r.authorize(ctx, id, attempt)
	if err != nil {
		return err
	}
	if err := r.flow.Engine.UpdateCapabilityTask(ctx, jobdomain.CapabilityTaskResult{
		WorkflowInstanceID: dispatch.ConductorWorkflowID, TaskID: dispatch.ConductorTaskID, WorkerID: r.instanceID,
		Status: "IN_PROGRESS", CallbackAfterSeconds: int(r.flow.lease().Seconds()),
	}); err != nil {
		return err
	}
	if _, err := r.flow.requireActiveDispatch(ctx, dispatch, attempt); err != nil {
		return err
	}
	_, err = r.flow.Dispatches.ExtendLease(ctx, id, attempt, r.flow.now().Add(r.flow.lease()))
	return err
}

func (r *ManagedRuntime) Complete(ctx context.Context, id string, attempt int, idempotencyKey string, output map[string]any) (*jobdomain.WorkerResultReceipt, error) {
	if !validCandidates(output["candidates"]) {
		return nil, ErrInvalidOutput
	}
	dispatch, err := r.authorizeResultIdentity(ctx, id, attempt)
	if err != nil {
		return nil, err
	}
	return r.flow.deliverResult(ctx, dispatch, "complete", idempotencyKey, output)
}

func (r *ManagedRuntime) Fail(ctx context.Context, id string, attempt int, idempotencyKey, reason string) (*jobdomain.WorkerResultReceipt, error) {
	return r.FailWithKind(ctx, id, attempt, idempotencyKey, reason, FailurePermanent)
}

func (r *ManagedRuntime) FailWithKind(ctx context.Context, id string, attempt int, idempotencyKey, reason, kind string) (*jobdomain.WorkerResultReceipt, error) {
	result, err := failureResult(reason, kind)
	if err != nil {
		return nil, err
	}
	dispatch, err := r.authorizeResultIdentity(ctx, id, attempt)
	if err != nil {
		return nil, err
	}
	return r.flow.deliverResult(ctx, dispatch, "fail", idempotencyKey, result)
}

// LeaseInterval is used by the internal runner to renew a dispatch during work.
func (r *ManagedRuntime) LeaseInterval() time.Duration { return r.flow.lease() / 3 }
