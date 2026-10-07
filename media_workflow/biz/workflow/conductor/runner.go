package conductor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"media_agent/media_workflow/biz/shared/taskcontext"
	"media_agent/media_workflow/biz/shared/taskerror"
)

type Handler func(context.Context, map[string]any) (map[string]any, error)

type Runner struct {
	Client       *Client
	WorkerID     string
	PollInterval time.Duration
	Handlers     map[string]Handler
	TaskDomains  map[string]string
}

func (r *Runner) Run(ctx context.Context) error {
	if r.Client == nil || !r.Client.Enabled() {
		return fmt.Errorf("conductor client is not configured")
	}
	if r.WorkerID == "" {
		return fmt.Errorf("worker id is required")
	}
	if len(r.Handlers) == 0 {
		return fmt.Errorf("at least one task handler is required")
	}
	interval := r.PollInterval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	errCh := make(chan error, len(r.Handlers))
	workerCtx, cancelWorkers := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancelWorkers(); workers.Wait() }()
	for taskType, handler := range r.Handlers {
		workers.Add(1)
		go func(taskType string, handler Handler) {
			defer workers.Done()
			errCh <- r.runTaskType(workerCtx, taskType, handler, interval)
		}(taskType, handler)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (r *Runner) runTaskType(ctx context.Context, taskType string, handler Handler, interval time.Duration) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		task, err := r.Client.Poll(ctx, taskType, r.WorkerID, r.TaskDomains[taskType])
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("event=internal_poll_deferred task_type=%s", taskType)
			if !wait(ctx, interval) {
				return ctx.Err()
			}
			continue
		}
		if task == nil {
			if !wait(ctx, interval) {
				return ctx.Err()
			}
			continue
		}
		// Read-only optional metadata works with already published definitions.
		metadataCtx, cancelMetadata := context.WithTimeout(ctx, 2*time.Second)
		execution, metadataErr := r.Client.GetWorkflow(metadataCtx, task.WorkflowInstanceID)
		cancelMetadata()
		var envelope map[string]any
		if metadataErr == nil && execution != nil && execution.WorkflowID == task.WorkflowInstanceID {
			envelope = execution.Input
		}
		taskCtx, span := taskcontext.StartExecution(ctx, envelope, "workflow.internal_task")
		taskCtx = taskcontext.WithIdentity(taskCtx, taskcontext.Identity{WorkflowID: task.WorkflowInstanceID, TaskID: task.TaskID, TaskType: task.TaskType, Reference: task.ReferenceTaskName})
		jobID, _ := task.InputData["job_id"].(string)
		taskcontext.Log(taskCtx, taskcontext.Event{Event: "internal_task_started", JobID: jobID, WorkflowID: task.WorkflowInstanceID, TaskID: task.TaskID, TaskType: taskType})
		output, executeErr := handler(taskCtx, task.InputData)
		result := TaskResult{
			WorkflowInstanceID: task.WorkflowInstanceID, TaskID: task.TaskID, WorkerID: r.WorkerID,
			Status: "COMPLETED", OutputData: output,
		}
		if executeErr != nil {
			result.Status = "FAILED"
			if taskerror.IsPermanent(executeErr) {
				result.Status = "FAILED_WITH_TERMINAL_ERROR"
			}
			result.OutputData = nil
			result.ReasonForIncompletion = taskerror.Reason(executeErr)
		}
		if err := r.Client.UpdateTask(taskCtx, result); err != nil {
			span.End()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			taskcontext.Log(taskCtx, taskcontext.Event{Event: "internal_task_update_unconfirmed", JobID: jobID, WorkflowID: task.WorkflowInstanceID, TaskID: task.TaskID, TaskType: taskType, Status: result.Status})
		} else {
			taskcontext.Log(taskCtx, taskcontext.Event{Event: "internal_task_updated", JobID: jobID, WorkflowID: task.WorkflowInstanceID, TaskID: task.TaskID, TaskType: taskType, Status: result.Status})
			span.End()
		}
	}
}

func JSONHandler[Input any, Output any](execute func(Input) (Output, error)) Handler {
	return JSONHandlerContext(func(_ context.Context, input Input) (Output, error) { return execute(input) })
}

func JSONHandlerContext[Input any, Output any](execute func(context.Context, Input) (Output, error)) Handler {
	return func(ctx context.Context, raw map[string]any) (map[string]any, error) {
		encoded, err := json.Marshal(raw)
		if err != nil {
			return nil, taskerror.Permanent("internal_task_input_invalid")
		}
		var input Input
		if err := json.Unmarshal(encoded, &input); err != nil {
			return nil, taskerror.Permanent("internal_task_input_invalid")
		}
		output, err := execute(ctx, input)
		if err != nil {
			return nil, err
		}
		encoded, err = json.Marshal(output)
		if err != nil {
			return nil, err
		}
		var result map[string]any
		if err := json.Unmarshal(encoded, &result); err != nil {
			return nil, err
		}
		return result, nil
	}
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
