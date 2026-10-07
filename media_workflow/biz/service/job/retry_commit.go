package job

import (
	"context"
	"errors"
	"time"

	domain "media_agent/media_workflow/biz/domain/job"
)

func (s *Service) commitNativeJobRetry(ctx context.Context, expected domain.Job, reason string, inputOverride *domain.JobInput) error {
	commit := func(txCtx context.Context) error {
		if s.reviews != nil {
			if err := s.reviews.LockReviewProjection(txCtx, expected.ID, ""); err != nil {
				return err
			}
		}
		current, err := s.jobRepo.Get(txCtx, expected.ID)
		if err != nil {
			return err
		}
		if current == nil || current.Status != domain.JobFailed || current.ConductorWorkflowID != expected.ConductorWorkflowID || current.CompilationHash != expected.CompilationHash || current.RetryCount != expected.RetryCount {
			return errors.New("retry snapshot superseded")
		}
		steps, err := s.stepRunRepo.ListByJob(txCtx, expected.ID)
		if err != nil {
			return err
		}
		if inputOverride != nil {
			if err := s.jobRepo.UpdateInput(txCtx, expected.ID, *inputOverride); err != nil {
				return err
			}
		}
		for _, step := range steps {
			if s.reviews != nil {
				if err := s.reviews.LockReviewProjection(txCtx, expected.ID, step.ID); err != nil {
					return err
				}
			}
			if err := s.stepRunRepo.UpdateCandidates(txCtx, step.ID, []domain.Candidate{}); err != nil {
				return err
			}
			if err := s.stepRunRepo.UpdateStatus(txCtx, step.ID, domain.SRBlocked, map[string]any{"selected_index": nil, "failure_reason": "", "output_asset_id": "", "claimed_by": "", "claimed_at": nil, "started_at": nil, "finished_at": nil}); err != nil {
				return err
			}
		}
		if err := s.jobRepo.UpdateStatus(txCtx, expected.ID, domain.JobRunning, ""); err != nil {
			return err
		}
		return s.jobRepo.AppendRetryHistory(txCtx, expected.ID, domain.RetryRecord{RetryAt: time.Now(), Reason: reason, InputOverride: inputOverride}, expected.RetryCount+1)
	}
	if s.reviews != nil && s.tx == nil {
		return errors.New("retry transaction unavailable")
	}
	var err error
	if s.tx != nil {
		err = s.tx.RunInTx(ctx, commit)
	} else {
		err = commit(ctx)
	}
	if err != nil {
		return errors.New("retry local commit unavailable; query existing execution before retrying")
	}
	return nil
}
