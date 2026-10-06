package conductor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	jobdomain "media_agent/media_workflow/biz/domain/job"
)

type ExecutionEngine struct{ Client *Client }

func (e *ExecutionEngine) FindStartedExecution(ctx context.Context, command jobdomain.ExecutionStart) (string, error) {
	if command.Validate() != nil {
		return "", errors.New("invalid frozen start reconciliation command")
	}
	var input map[string]any
	var domains map[string]string
	if json.Unmarshal(command.WorkflowInput, &input) != nil || json.Unmarshal(command.TaskDomains, &domains) != nil {
		return "", errors.New("invalid frozen start reconciliation payload")
	}
	return e.findExactStart(ctx, command.WorkflowName, command.WorkflowVersion, command.CorrelationID, input, domains)
}

func (e *ExecutionEngine) findExactStart(ctx context.Context, name string, version int, correlation string, input map[string]any, domains map[string]string) (string, error) {
	id, err := e.Client.GetByCorrelation(ctx, name, correlation)
	if err != nil {
		return "", errors.New("start correlation reconciliation unavailable or conflicting")
	}
	if id == "" {
		return "", nil
	}
	execution, err := e.Client.GetWorkflow(ctx, id)
	if err != nil || execution == nil {
		return "", errors.New("start execution reconciliation unavailable")
	}
	equalJSON := func(a, b any) bool {
		left, lerr := json.Marshal(a)
		right, rerr := json.Marshal(b)
		return lerr == nil && rerr == nil && bytes.Equal(left, right)
	}
	// Empty task domains are represented as {} by OSS, even if omitted from
	// the original POST. Nonempty routing must match exactly.
	if domains == nil {
		domains = map[string]string{}
	}
	if execution.WorkflowID != id || execution.CorrelationID != correlation || execution.WorkflowDefinition.Name != name || execution.WorkflowDefinition.Version != version || execution.Input == nil || execution.TaskToDomain == nil || !equalJSON(input, execution.Input) || !equalJSON(domains, execution.TaskToDomain) {
		return "", errors.New("start execution conflicts with frozen command")
	}
	return id, nil
}

func (e *ExecutionEngine) Start(ctx context.Context, workflowName string, workflowVersion int, correlationID string, input map[string]any, taskToDomain map[string]string) (string, error) {
	id, err := e.Client.StartWorkflow(ctx, StartWorkflowRequest{
		Name: workflowName, Version: workflowVersion, CorrelationID: correlationID,
		Input: input, TaskToDomain: taskToDomain,
	})
	if err == nil {
		return id, nil
	}
	// A lost Start response is ambiguous. Correlation can recover a single
	// existing execution, but does not authorize issuing another Start: the
	// deployed OSS server ignores RETURN_EXISTING on this endpoint.
	if correlationID != "" && ctx.Err() == nil {
		if existing, lookupErr := e.findExactStart(ctx, workflowName, workflowVersion, correlationID, input, taskToDomain); lookupErr == nil && existing != "" {
			return existing, nil
		}
	}
	return "", errors.New("conductor start outcome unconfirmed; do not automatically resubmit")
}

func (e *ExecutionEngine) Get(ctx context.Context, workflowID string) (*jobdomain.EngineExecution, error) {
	execution, err := e.Client.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	result := &jobdomain.EngineExecution{Status: execution.Status, Output: execution.Output}
	for _, task := range execution.Tasks {
		result.Tasks = append(result.Tasks, jobdomain.EngineTask{
			Sequence: task.Sequence,
			TaskID:   task.TaskID, TaskType: task.TaskType, ReferenceTaskName: task.ReferenceTaskName, Status: task.Status,
			Input: task.InputData, Output: task.OutputData,
		})
	}
	return result, nil
}

func (e *ExecutionEngine) CompleteTask(ctx context.Context, workflowID, taskReferenceName string, output map[string]any) error {
	return e.Client.CompleteTaskByReference(ctx, workflowID, taskReferenceName, output)
}

func (e *ExecutionEngine) CompleteApprovalTask(ctx context.Context, workflowID, taskID string, output map[string]any) error {
	if workflowID == "" || taskID == "" {
		return errors.New("exact approval identity required")
	}
	return e.Client.UpdateTask(ctx, TaskResult{WorkflowInstanceID: workflowID, TaskID: taskID, WorkerID: "media-workflow-review", Status: "COMPLETED", OutputData: output})
}

func (e *ExecutionEngine) RerunFromTask(ctx context.Context, workflowID, taskID string) error {
	return e.Client.RerunFromTask(ctx, workflowID, taskID)
}

func (e *ExecutionEngine) RerunWorkflow(ctx context.Context, workflowID string, input map[string]any) error {
	return e.Client.RerunWorkflow(ctx, workflowID, input)
}

func (e *ExecutionEngine) Terminate(ctx context.Context, workflowID, reason string) error {
	return e.Client.TerminateWorkflow(ctx, workflowID, reason)
}

func (e *ExecutionEngine) PollCapabilityTask(ctx context.Context, taskType, workerID, executionDomain string) (*jobdomain.CapabilityTask, error) {
	task, err := e.Client.Poll(ctx, taskType, workerID, executionDomain)
	if err != nil || task == nil {
		return nil, err
	}
	return &jobdomain.CapabilityTask{
		TaskType: task.TaskType, ReferenceTaskName: task.ReferenceTaskName,
		TaskID: task.TaskID, WorkflowInstanceID: task.WorkflowInstanceID,
		RetryCount: task.RetryCount, Input: task.InputData,
	}, nil
}

func (e *ExecutionEngine) UpdateCapabilityTask(ctx context.Context, result jobdomain.CapabilityTaskResult) error {
	return e.Client.UpdateTask(ctx, TaskResult{
		WorkflowInstanceID: result.WorkflowInstanceID, TaskID: result.TaskID,
		WorkerID: result.WorkerID, Status: result.Status, OutputData: result.Output,
		ReasonForIncompletion: result.Reason, CallbackAfterSeconds: result.CallbackAfterSeconds,
	})
}

var _ jobdomain.WorkflowEngine = (*ExecutionEngine)(nil)
var _ jobdomain.ExecutionStartEngine = (*ExecutionEngine)(nil)
var _ jobdomain.CapabilityTaskEngine = (*ExecutionEngine)(nil)
