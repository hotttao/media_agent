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
			TaskType: task.TaskType, ReferenceTaskName: task.ReferenceTaskName, Status: task.Status,
			Input: task.InputData, Output: task.OutputData,
		})
	}
	return result, nil
}

var _ jobdomain.WorkflowEngine = (*ExecutionEngine)(nil)
