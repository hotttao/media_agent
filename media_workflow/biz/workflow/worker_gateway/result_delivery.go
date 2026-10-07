package worker_gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	jobdomain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/taskcontext"
)

var ErrResultUnconfirmed = errors.New("native task result unconfirmed; retry the same result")

// No durable result queue: the caller keeps the same output until the original
// native task acknowledges it. A successful HTTP update alone is insufficient:
// Conductor can ignore updates to an already terminal task.
func (f *Flow) deliverResult(ctx context.Context, dispatch *jobdomain.WorkerDispatch, operation, key string, result map[string]any) (*jobdomain.WorkerResultReceipt, error) {
	if key == "" || dispatch == nil || (operation != "complete" && operation != "fail") {
		return nil, ErrInvalidRequest
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, ErrInvalidOutput
	}
	status, reason, output := "COMPLETED", "", result
	if operation == "fail" {
		kind, _ := result["failure_kind"].(string)
		status, err = failureStatus(kind)
		if err != nil {
			return nil, err
		}
		reason, _ = result["reason"].(string)
		if reason == "" {
			return nil, ErrInvalidRequest
		}
		output = nil
	}
	job, err := f.Jobs.Get(ctx, dispatch.JobID)
	if err != nil {
		return nil, ErrResultUnconfirmed
	}
	if job == nil || job.ID != dispatch.JobID || job.ConductorWorkflowID != dispatch.ConductorWorkflowID || job.OrganizationID != dispatch.OrganizationID || job.SubmittedBy != dispatch.UserID {
		return nil, ErrForbidden
	}
	observe := func() (*jobdomain.EngineExecution, *jobdomain.EngineTask, error) {
		if f.Executions == nil {
			return nil, nil, ErrResultUnconfirmed
		}
		execution, err := f.Executions.Get(ctx, dispatch.ConductorWorkflowID)
		if err != nil || execution == nil {
			return nil, nil, ErrResultUnconfirmed
		}
		if execution.WorkflowInstanceID != dispatch.ConductorWorkflowID {
			return nil, nil, ErrForbidden
		}
		var found *jobdomain.EngineTask
		for i := range execution.Tasks {
			task := &execution.Tasks[i]
			if task.TaskID != dispatch.ConductorTaskID {
				continue
			}
			if found != nil || task.Input["job_id"] != job.ID || task.RetryCount+1 != dispatch.Attempt {
				return nil, nil, ErrForbidden
			}
			found = task
		}
		if found == nil {
			return nil, nil, jobdomain.ErrDispatchExpired
		}
		return execution, found, nil
	}
	acknowledged := func(task *jobdomain.EngineTask) bool {
		if task.Status != status {
			return false
		}
		if operation == "fail" {
			return task.ReasonForIncompletion == reason
		}
		actual, err := json.Marshal(task.Output)
		return err == nil && string(actual) == string(encoded)
	}
	execution, task, err := observe()
	if err != nil {
		return nil, err
	}
	if !acknowledged(task) {
		if terminalTask(task.Status) {
			return nil, jobdomain.ErrDispatchConflict
		}
		if job.Status == jobdomain.JobCancelled || execution.Status != "RUNNING" || task.Retried || (task.Status != "IN_PROGRESS" && task.Status != "SCHEDULED") {
			return nil, jobdomain.ErrDispatchExpired
		}
		if operation == "complete" {
			if err := f.validateOutputMedia(ctx, dispatch, job, result); err != nil {
				return nil, err
			}
		}
		// After any response, read the exact task. Lost successful responses
		// are resolved without re-executing the provider or resending blindly.
		_ = f.Engine.UpdateCapabilityTask(ctx, jobdomain.CapabilityTaskResult{WorkflowInstanceID: dispatch.ConductorWorkflowID, TaskID: dispatch.ConductorTaskID, WorkerID: "media-workflow-proxy", Status: status, Output: output, Reason: reason})
		_, task, err = observe()
		if err != nil {
			return nil, err
		}
		if !acknowledged(task) {
			if terminalTask(task.Status) {
				return nil, jobdomain.ErrDispatchConflict
			}
			return nil, ErrResultUnconfirmed
		}
	}
	if err := f.Dispatches.MarkResultReceived(ctx, dispatch.ID, dispatch.Attempt, f.now()); err != nil {
		return nil, ErrResultUnconfirmed
	}
	taskcontext.Log(ctx, taskcontext.Event{Event: "worker_result_confirmed", JobID: dispatch.JobID, DispatchID: dispatch.ID, TaskID: dispatch.ConductorTaskID, Status: status})
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", dispatch.ID, dispatch.Attempt)))
	return &jobdomain.WorkerResultReceipt{ID: "ack_" + hex.EncodeToString(digest[:]), DispatchID: dispatch.ID, Attempt: dispatch.Attempt, Operation: operation, IdempotencyKey: key, ResultJSON: encoded, ConductorStatus: "submitted", Dispatch: dispatch}, nil
}

func terminalTask(status string) bool {
	switch status {
	case "COMPLETED", "FAILED", "FAILED_WITH_TERMINAL_ERROR", "TIMED_OUT", "CANCELED", "SKIPPED":
		return true
	default:
		return false
	}
}
