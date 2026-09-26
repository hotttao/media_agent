package conductor

import (
	"context"

	jobdomain "media_agent/media_workflow/biz/domain/job"
)

type ExecutionEngine struct{ Client *Client }

func (e *ExecutionEngine) Start(ctx context.Context, workflowName string, workflowVersion int, input map[string]any) (string, error) {
	return e.Client.StartWorkflow(ctx, workflowName, workflowVersion, input)
}

func (e *ExecutionEngine) Get(ctx context.Context, workflowID string) (*jobdomain.EngineExecution, error) {
	execution, err := e.Client.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	result := &jobdomain.EngineExecution{Status: execution.Status, Output: execution.Output}
	for _, task := range execution.Tasks {
		result.Tasks = append(result.Tasks, jobdomain.EngineTask{
			TaskID: task.TaskID, TaskType: task.TaskType, ReferenceTaskName: task.ReferenceTaskName, Status: task.Status,
			Input: task.InputData, Output: task.OutputData,
		})
	}
	return result, nil
}

func (e *ExecutionEngine) CompleteTask(ctx context.Context, workflowID, taskReferenceName string, output map[string]any) error {
	return e.Client.CompleteTaskByReference(ctx, workflowID, taskReferenceName, output)
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

var _ jobdomain.WorkflowEngine = (*ExecutionEngine)(nil)
