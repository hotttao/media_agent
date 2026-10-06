// Package review_delivery submits durable approval decisions without holding
// a database transaction across Conductor calls.
package review_delivery

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	domain "media_agent/media_workflow/biz/domain/job"
)

var ErrTemporarilyUnavailable = errors.New("approval delivery temporarily unavailable")

type Flow struct {
	Reviews domain.ReviewRepo
	Jobs    domain.JobRepo
	Engine  domain.WorkflowEngine
}

func (f *Flow) DispatchOne(ctx context.Context) error {
	if f.Reviews == nil || f.Jobs == nil || f.Engine == nil {
		return errors.New("approval delivery is not configured")
	}
	review, err := f.Reviews.ClaimPendingReview(ctx)
	if err != nil || review == nil {
		return err
	}
	if review.Validate() != nil {
		return f.Reviews.FinishReview(ctx, *review, "failed", "invalid_frozen_decision")
	}
	release := func(code string) error {
		if err := f.Reviews.ReleaseReviewClaim(ctx, *review, code); err != nil {
			return err
		}
		return ErrTemporarilyUnavailable // Never return/log the upstream body.
	}
	job, err := f.Jobs.Get(ctx, review.JobID)
	if err != nil {
		return release("job_unavailable")
	}
	if job.ConductorWorkflowID != review.ConductorWorkflowID || job.SubmittedBy != review.DecidedBy {
		return f.Reviews.FinishReview(ctx, *review, "failed", "execution_replaced")
	}
	if review.AcceptedRetryCount != nil && *review.AcceptedRetryCount != job.RetryCount {
		return f.Reviews.FinishReview(ctx, *review, "failed", "execution_retried")
	}
	execution, err := f.Engine.Get(ctx, review.ConductorWorkflowID)
	if err != nil || execution == nil {
		return release("execution_unavailable")
	}
	var target *domain.EngineTask
	for i := range execution.Tasks {
		if execution.Tasks[i].TaskID == review.ConductorTaskID {
			target = &execution.Tasks[i]
			break
		}
	}
	if target == nil || target.ReferenceTaskName != review.TaskReferenceName || target.TaskType != "HUMAN" {
		return f.Reviews.FinishReview(ctx, *review, "failed", "approval_task_replaced")
	}
	var output map[string]any
	if json.Unmarshal(review.OutputJSON, &output) != nil {
		return f.Reviews.FinishReview(ctx, *review, "failed", "invalid_frozen_output")
	}
	if target.Status == "COMPLETED" {
		// A committed submission can lose its HTTP response. Querying the exact
		// task closes that window without issuing another decision command.
		encoded, err := json.Marshal(target.Output)
		var actual map[string]any
		if err != nil || json.Unmarshal(encoded, &actual) != nil || !reflect.DeepEqual(output, actual) {
			return f.Reviews.FinishReview(ctx, *review, "failed", "approval_output_conflict")
		}
		return f.Reviews.FinishReview(ctx, *review, "applied", "")
	}
	if job.Status.IsTerminal() || execution.Status != "RUNNING" || target.Status != "IN_PROGRESS" {
		return f.Reviews.FinishReview(ctx, *review, "failed", "approval_no_longer_waiting")
	}
	if review.AcceptedRetryCount == nil && job.RetryCount > 0 {
		return f.Reviews.FinishReview(ctx, *review, "failed", "unknown_retry_epoch")
	}
	// References may be reused by a concurrent Retry after the lookup. Submit
	// the frozen Task ID, never resolve the reference to a newer HUMAN again.
	exact, ok := f.Engine.(domain.ApprovalTaskEngine)
	if !ok {
		return release("exact_approval_submission_unavailable")
	}
	if err := exact.CompleteApprovalTask(ctx, review.ConductorWorkflowID, review.ConductorTaskID, output); err != nil {
		return release("task_completion_unavailable")
	}
	return f.Reviews.FinishReview(ctx, *review, "applied", "")
}
