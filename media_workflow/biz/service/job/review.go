package job

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"media_agent/hertz_infra/authn/internaljwt"
	domain "media_agent/media_workflow/biz/domain/job"
)

type ReviewCommand struct {
	StepRunID, UserID, OrganizationID, IdempotencyKey, Decision, Feedback string
	SelectedIndex                                                         int
}

func (s *Service) ConfigureReviews(repo domain.ReviewRepo) { s.reviews = repo }

// DecideReview freezes a decision locally. Only the delivery flow writes to
// Conductor; a failed HTTP response must not trigger a second generation.
func (s *Service) DecideReview(ctx context.Context, cmd ReviewCommand) (*domain.StepRun, error) {
	if s.reviews == nil || s.engine == nil || s.tx == nil || s.resumeAccess == nil {
		return nil, ErrSubmitAuthorityUnavailable
	}
	if cmd.Decision != "accept" && cmd.Decision != "regenerate" {
		return nil, errors.New("invalid review decision")
	}
	if len(cmd.Feedback) > 8192 || (cmd.Decision == "accept" && cmd.Feedback != "") {
		return nil, errors.New("invalid review feedback")
	}
	key := strings.TrimSpace(cmd.IdempotencyKey)
	if len(key) > 160 {
		return nil, errors.New("idempotency_key exceeds 160 bytes")
	}
	sr, err := s.stepRunRepo.Get(ctx, cmd.StepRunID)
	if err != nil {
		return nil, err
	}
	j, err := s.jobRepo.Get(ctx, sr.JobID)
	if err != nil {
		return nil, err
	}
	if cmd.UserID == "" || j.SubmittedBy != cmd.UserID || j.WorkspaceID == "" ||
		(cmd.OrganizationID != "" && j.OrganizationID != cmd.OrganizationID) {
		return nil, ErrSubmitForbidden
	}
	if principal, ok := internaljwt.AgentPrincipalFromContext(ctx); ok &&
		(principal.UserID != cmd.UserID || principal.OrganizationID != j.OrganizationID || principal.WorkspaceID != j.WorkspaceID) {
		return nil, ErrSubmitForbidden
	}
	allowed, err := s.resumeAccess.CanUseWorkflow(ctx, j.WorkspaceID, cmd.UserID)
	if err != nil {
		return nil, ErrSubmitAuthorityUnavailable
	}
	if !allowed {
		return nil, ErrSubmitForbidden
	}
	// Explicit keys replay even after the workflow has advanced or terminated,
	// without consulting Conductor or selecting a newer approval round.
	if key != "" {
		previous, err := s.reviews.GetReviewByKey(ctx, sr.ID, key)
		if err != nil {
			return nil, err
		}
		if previous != nil {
			if previous.DecidedBy != cmd.UserID || previous.Decision != cmd.Decision || previous.Feedback != cmd.Feedback ||
				(cmd.Decision == "accept" && (previous.SelectedIndex == nil || *previous.SelectedIndex != cmd.SelectedIndex)) {
				return nil, domain.ErrReviewConflict
			}
			return reviewReceipt(sr, previous), nil
		}
	}
	if sr.Status != domain.SRNeedConfirm {
		return nil, domain.ErrReviewConflict
	}
	task, generation, err := s.currentApprovalTask(ctx, j, sr)
	if err != nil {
		return nil, err
	}
	if task.TaskID == "" || task.TaskType != "HUMAN" {
		return nil, domain.ErrReviewConflict
	}
	if cmd.Feedback != "" && task.Input["feedback_mode"] != "last_regeneration_v1" {
		return nil, errors.New("this published workflow does not support regeneration feedback; publish a new version")
	}
	if key == "" {
		// Compatibility for older callers: deduplicate the currently observed
		// round only. New callers should always send their own stable key.
		key = "approval:" + task.TaskID
	}
	snapshot, err := json.Marshal(task.Input["candidates"])
	if err != nil {
		return nil, domain.ErrReviewConflict
	}
	review := domain.ReviewDecision{ID: "review_" + uuid.NewString(), JobID: j.ID, StepRunID: sr.ID,
		ConductorWorkflowID: j.ConductorWorkflowID, ConductorTaskID: task.TaskID, TaskReferenceName: task.ReferenceTaskName,
		Generation: generation, Decision: cmd.Decision, DecidedBy: cmd.UserID, IdempotencyKey: key,
		Feedback: cmd.Feedback, CandidatesSnapshot: snapshot, Status: "pending"}
	retryEpoch := j.RetryCount
	review.AcceptedRetryCount = &retryEpoch
	if !review.MatchesCandidates(sr.Candidates) {
		return nil, domain.ErrReviewConflict
	}
	output := map[string]any{"decision": cmd.Decision}
	if cmd.Decision == "accept" {
		if cmd.SelectedIndex < 0 || cmd.SelectedIndex >= len(sr.Candidates) {
			return nil, errors.New("selected_index out of range")
		}
		index := cmd.SelectedIndex
		review.SelectedIndex = &index
		output["selected_index"], output["selected_candidate"] = index, sr.Candidates[index]
	} else if generation == 0 {
		return nil, errors.New("this published workflow has no regeneration loop; publish a new version")
	}
	if cmd.Decision == "regenerate" && cmd.Feedback != "" {
		output["feedback"] = cmd.Feedback
	}
	review.OutputJSON, err = json.Marshal(output)
	if err != nil {
		return nil, err
	}
	review.RequestDigest, err = review.Digest()
	if err != nil {
		return nil, err
	}
	var accepted *domain.ReviewDecision
	err = s.tx.RunInTx(ctx, func(txCtx context.Context) error {
		var err error
		accepted, _, err = s.reviews.AcceptReview(txCtx, review)
		return err
	})
	if err != nil {
		return nil, err
	}
	sr, err = s.stepRunRepo.Get(ctx, sr.ID)
	if err != nil {
		return nil, err
	}
	return reviewReceipt(sr, accepted), nil
}

func reviewReceipt(sr *domain.StepRun, review *domain.ReviewDecision) *domain.StepRun {
	copy := *sr
	copy.LastReviewID, copy.LastReviewStatus = review.ID, review.Status
	return &copy
}

func (s *Service) annotateReviews(ctx context.Context, jobID string, steps []*domain.StepRun) error {
	if s.reviews == nil {
		return nil
	}
	reviews, err := s.reviews.LatestReviewsByJob(ctx, jobID)
	if err != nil {
		return err
	}
	byStep := map[string]domain.ReviewDecision{}
	for _, review := range reviews {
		byStep[review.StepRunID] = review
	}
	for _, step := range steps {
		if review, ok := byStep[step.ID]; ok {
			step.LastReviewID, step.LastReviewStatus = review.ID, review.Status
		}
	}
	return nil
}

func (s *Service) projectWaitingReview(ctx context.Context, jobID, stepID string, task domain.EngineTask) error {
	return s.tx.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.reviews.LockReviewProjection(txCtx, jobID, stepID); err != nil {
			return err
		}
		j, err := s.jobRepo.Get(txCtx, jobID)
		if err != nil {
			return err
		}
		if j.Status.IsTerminal() {
			return nil
		}
		if err := checkExecutionSnapshot(txCtx, j); err != nil {
			return err
		}
		if err := s.checkGenerationSnapshot(txCtx, jobID, stepID); err != nil {
			return err
		}
		step, err := s.stepRunRepo.Get(txCtx, stepID)
		if err != nil {
			return err
		}
		if step.Status == domain.SRSucceeded && step.OutputAssetID != "" {
			return nil
		}
		review, err := s.reviews.GetReviewByTask(txCtx, stepID, task.TaskID)
		if err != nil || review != nil {
			return err
		}
		var candidates []domain.Candidate
		encoded, err := json.Marshal(task.Input["candidates"])
		if err != nil || json.Unmarshal(encoded, &candidates) != nil || len(candidates) == 0 {
			return errors.New("invalid approval candidates")
		}
		if err := s.stepRunRepo.UpdateCandidates(txCtx, stepID, candidates); err != nil {
			return err
		}
		return s.stepRunRepo.UpdateStatus(txCtx, stepID, domain.SRNeedConfirm, nil)
	})
}

func (s *Service) refreshReviewJobStatus(ctx context.Context, jobID string) error {
	return s.tx.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.reviews.LockReviewProjection(txCtx, jobID, ""); err != nil {
			return err
		}
		j, steps, err := s.jobRepo.GetWithStepRuns(txCtx, jobID)
		if err != nil {
			return err
		}
		if j.Status.IsTerminal() {
			return nil
		}
		if err := checkExecutionSnapshot(txCtx, j); err != nil {
			return err
		}
		if err := s.checkGenerationSnapshot(txCtx, jobID, ""); err != nil {
			return err
		}
		status := domain.JobRunning
		for _, step := range steps {
			if step.Status == domain.SRNeedConfirm {
				status = domain.JobNeedConfirm
				break
			}
		}
		if j.Status == status {
			return nil
		}
		return s.jobRepo.UpdateStatus(txCtx, jobID, status, "")
	})
}
