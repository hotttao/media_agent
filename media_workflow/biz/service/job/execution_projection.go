package job

import (
	"context"
	"errors"

	domain "media_agent/media_workflow/biz/domain/job"
)

var errExecutionSnapshotStale = errors.New("execution snapshot superseded by retry")

type executionSnapshotKey struct{}
type executionSnapshot struct {
	jobID, engineID string
	retryCount      int
}

func checkExecutionSnapshot(ctx context.Context, current *domain.Job) error {
	// Cancellation intent is guarded separately; cached failures are refreshable.
	if current.Status == domain.JobCancelled {
		return nil
	}
	if snapshot, ok := ctx.Value(executionSnapshotKey{}).(executionSnapshot); ok && snapshot.jobID == current.ID &&
		(snapshot.engineID != current.ConductorWorkflowID || snapshot.retryCount != current.RetryCount) {
		return errExecutionSnapshotStale
	}
	return nil
}

// withExecutionProjection serializes local projections with accepted review and
// cancellation commands. Its callback must not perform external calls.
func (s *Service) withExecutionProjection(ctx context.Context, jobID, stepID string, fn func(context.Context, *domain.Job) error) error {
	project := func(txCtx context.Context) error {
		if s.reviews != nil {
			if err := s.reviews.LockReviewProjection(txCtx, jobID, stepID); err != nil {
				return err
			}
		}
		current, err := s.jobRepo.Get(txCtx, jobID)
		if err != nil {
			return err
		}
		if err := checkExecutionSnapshot(txCtx, current); err != nil {
			return err
		}
		if current.Status != domain.JobCancelled {
			if err := s.checkGenerationSnapshot(txCtx, jobID, stepID); err != nil {
				return err
			}
		}
		return fn(txCtx, current)
	}
	if s.reviews != nil && s.tx == nil {
		return errors.New("execution projection transaction unavailable")
	}
	if s.tx != nil {
		return s.tx.RunInTx(ctx, project)
	}
	// Legacy callers without the native review repository have no lock port.
	return project(ctx)
}

func (s *Service) projectExecutionStatus(ctx context.Context, jobID string, status domain.JobStatus, reason string) error {
	return s.withExecutionProjection(ctx, jobID, "", func(txCtx context.Context, current *domain.Job) error {
		if current.Status == domain.JobCancelled {
			return nil
		}
		if current.Status == status && current.FailureReason == reason {
			return nil
		}
		return s.jobRepo.UpdateStatus(txCtx, jobID, status, reason)
	})
}

func (s *Service) withActiveStepProjection(ctx context.Context, jobID, stepID string, fn func(context.Context) error) error {
	return s.withExecutionProjection(ctx, jobID, stepID, func(txCtx context.Context, current *domain.Job) error {
		if current.Status == domain.JobCancelled {
			return nil
		}
		step, err := s.stepRunRepo.Get(txCtx, stepID)
		if err != nil {
			return err
		}
		// Native retry preserves successful tasks. A delayed snapshot must not
		// overwrite their business output while another branch is still running.
		if step.Status == domain.SRSucceeded && step.OutputAssetID != "" {
			return nil
		}
		return fn(txCtx)
	})
}
