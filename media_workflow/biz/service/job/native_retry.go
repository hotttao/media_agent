package job

import (
	"context"
	"errors"
	"time"

	domain "media_agent/media_workflow/biz/domain/job"
)

// continueExecution delegates recovery to the existing engine instance. Local
// retry_count is a legacy rerun epoch, not permission to schedule tasks.
func (s *Service) continueExecution(ctx context.Context, job *domain.Job, reason string) (*domain.Job, error) {
	if job.Status == domain.JobCancelled {
		return nil, errors.New("cancelled execution cannot use failure retry")
	}
	engine, ok := s.engine.(domain.FailureRetryEngine)
	if !ok || job.ConductorWorkflowID == "" {
		return nil, errors.New("conductor failure retry unavailable")
	}
	execution, err := s.engine.Get(ctx, job.ConductorWorkflowID)
	if err != nil || execution == nil {
		return nil, errors.New("conductor execution unavailable")
	}
	if execution.WorkflowInstanceID != "" && execution.WorkflowInstanceID != job.ConductorWorkflowID {
		return nil, errors.New("conductor execution identity conflict")
	}
	switch execution.Status {
	case "RUNNING":
		// A repeated request observes recovery; never post a second retry.
	case "FAILED", "TIMED_OUT":
		if err := engine.RetryWorkflow(ctx, job.ConductorWorkflowID); err != nil {
			// A lost response does not authorize another POST. Observe this same
			// instance; RUNNING proves it can already continue.
			observed, lookupErr := s.engine.Get(ctx, job.ConductorWorkflowID)
			if lookupErr != nil || observed == nil || observed.Status != "RUNNING" ||
				(observed.WorkflowInstanceID != "" && observed.WorkflowInstanceID != job.ConductorWorkflowID) {
				return nil, errors.New("conductor retry outcome unconfirmed; query the existing execution")
			}
		} else {
			// Audit only: keep the rerun epoch unchanged and do not reset state.
			if err := s.jobRepo.AppendRetryHistory(ctx, job.ID, domain.RetryRecord{RetryAt: time.Now(), Reason: reason}, job.RetryCount); err != nil {
				return nil, errors.New("conductor retry accepted but audit unavailable; query the existing execution")
			}
		}
	default:
		return nil, errors.New("execution is not failed or timed out; use the appropriate native operation")
	}
	updated, _, err := s.Get(ctx, job.ID)
	return updated, err
}
