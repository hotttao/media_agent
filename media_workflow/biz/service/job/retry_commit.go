package job

import (
	"context"
	"errors"
	"time"

	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/workflow/executable"
)

// commitNativeStepRetry contains local writes only. It does not make the
// preceding engine call durable; a Retry intent must close that separate gap.
func (s *Service) commitNativeStepRetry(ctx context.Context, expectedJob domain.Job, expectedStep domain.StepRun, reason string, impact *executable.RetryImpact, frozenSteps []*domain.StepRun) error {
	conflict := errors.New("retry local state superseded; query existing execution before retrying")
	commit := func(txCtx context.Context) error {
		if s.reviews != nil {
			if err := s.reviews.LockReviewProjection(txCtx, expectedJob.ID, ""); err != nil {
				return err
			}
		}
		current, err := s.jobRepo.Get(txCtx, expectedJob.ID)
		if err != nil {
			return err
		}
		if current == nil || current.Status != domain.JobFailed || current.ConductorWorkflowID != expectedJob.ConductorWorkflowID || current.CompilationHash != expectedJob.CompilationHash || current.RetryCount != expectedJob.RetryCount {
			return conflict
		}
		if s.reviews != nil {
			if err := s.reviews.LockReviewProjection(txCtx, expectedJob.ID, expectedStep.ID); err != nil {
				return err
			}
		}
		target, err := s.stepRunRepo.Get(txCtx, expectedStep.ID)
		if err != nil {
			return err
		}
		if target == nil || target.JobID != expectedJob.ID || target.StepKey != expectedStep.StepKey || target.RetryCount != expectedStep.RetryCount || (target.Status != domain.SRFailed && target.Status != domain.SRSkipped) {
			return conflict
		}
		reset := func(stepID string, fields map[string]any) error {
			fields["claimed_by"], fields["claimed_at"], fields["started_at"], fields["finished_at"] = "", nil, nil, nil
			if err := s.stepRunRepo.UpdateCandidates(txCtx, stepID, []domain.Candidate{}); err != nil {
				return err
			}
			return s.stepRunRepo.UpdateStatus(txCtx, stepID, domain.SRBlocked, fields)
		}
		if err := reset(expectedStep.ID, map[string]any{"retry_count": expectedStep.RetryCount + 1, "selected_index": nil, "failure_reason": "", "output_asset_id": ""}); err != nil {
			return err
		}
		if impact != nil {
			affected := make(map[string]bool, len(impact.Affected))
			for _, key := range impact.Affected {
				affected[key] = true
			}
			for _, step := range frozenSteps {
				if step.StepKey == expectedStep.StepKey || !affected[step.StepKey] {
					continue
				}
				if s.reviews != nil {
					if err := s.reviews.LockReviewProjection(txCtx, expectedJob.ID, step.ID); err != nil {
						return err
					}
				}
				if err := reset(step.ID, map[string]any{"selected_index": nil, "failure_reason": "", "output_asset_id": ""}); err != nil {
					return err
				}
			}
		} else {
			// Compatibility for callers without a frozen compilation; standard
			// native wiring always supplies one. No engine calls happen here.
			s.resetDownstream(txCtx, expectedJob.ID, expectedStep.StepKey)
		}
		return s.jobRepo.AppendRetryHistory(txCtx, expectedJob.ID, domain.RetryRecord{RetryAt: time.Now(), Reason: reason}, expectedJob.RetryCount+1)
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
