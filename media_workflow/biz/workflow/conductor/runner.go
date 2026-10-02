package conductor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

type Handler func(context.Context, map[string]any) (map[string]any, error)

type Runner struct {
	Client       *Client
	WorkerID     string
	PollInterval time.Duration
	Handlers     map[string]Handler
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
	for taskType, handler := range r.Handlers {
		go func(taskType string, handler Handler) {
			errCh <- r.runTaskType(ctx, taskType, handler, interval)
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
		task, err := r.Client.Poll(ctx, taskType, r.WorkerID, "")
		if err != nil {
			log.Printf("internal worker poll %s: %v", taskType, err)
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
		output, executeErr := handler(ctx, task.InputData)
		result := TaskResult{
			WorkflowInstanceID: task.WorkflowInstanceID, TaskID: task.TaskID, WorkerID: r.WorkerID,
			Status: "COMPLETED", OutputData: output,
		}
		if executeErr != nil {
			result.Status = "FAILED_WITH_TERMINAL_ERROR"
			result.ReasonForIncompletion = executeErr.Error()
		}
		if err := r.Client.UpdateTask(ctx, result); err != nil {
			log.Printf("internal worker update %s task=%s: %v", taskType, task.TaskID, err)
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
			return nil, err
		}
		var input Input
		if err := json.Unmarshal(encoded, &input); err != nil {
			return nil, err
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
