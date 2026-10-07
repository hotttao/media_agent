package worker_gateway

import (
	"context"
	"errors"

	jobdomain "media_agent/media_workflow/biz/domain/job"
)

// requireActiveTask verifies an execution-engine identity, not the cached Job
// state. It never updates, postpones, resets or schedules an engine task.
func (f *Flow) requireActiveTask(ctx context.Context, job *jobdomain.Job, engineID, taskID string, attempt int) error {
	if f.Executions == nil {
		return errors.New("execution verification unavailable")
	}
	if job.Status == jobdomain.JobCancelled {
		return jobdomain.ErrDispatchExpired
	}
	if job.ConductorWorkflowID != engineID {
		return ErrForbidden
	}
	execution, err := f.Executions.Get(ctx, engineID)
	if err != nil {
		return errors.New("execution verification unavailable")
	}
	if execution == nil || execution.WorkflowInstanceID != engineID || execution.Status != "RUNNING" {
		return jobdomain.ErrDispatchExpired
	}
	var matched *jobdomain.EngineTask
	for i := range execution.Tasks {
		task := &execution.Tasks[i]
		if task.TaskID != taskID {
			continue
		}
		if matched != nil {
			return ErrForbidden
		}
		matched = task
	}
	if matched == nil || matched.Input["job_id"] != job.ID || matched.Retried || matched.RetryCount+1 != attempt ||
		(matched.Status != "IN_PROGRESS" && matched.Status != "SCHEDULED") {
		return jobdomain.ErrDispatchExpired
	}
	return nil
}
