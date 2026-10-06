package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	domain "media_agent/media_workflow/biz/domain/job"
)

func (s *Service) ConfigureSubmissions(repo domain.SubmissionRepo) { s.submissions = repo }

func (s *Service) prepareSubmission(ctx context.Context, cmd SubmitCommand) (*domain.Submission, *domain.Job, error) {
	if s.submissions == nil || s.starts == nil || s.tx == nil || s.compilations == nil || s.engine == nil {
		return nil, nil, errors.New("idempotent workflow submission is not configured")
	}
	key := domain.SubmissionKey{SubmittedBy: cmd.SubmittedBy, OrganizationID: cmd.OrganizationID, WorkspaceID: cmd.WorkspaceID, IdempotencyKey: cmd.IdempotencyKey}
	if err := key.Validate(); err != nil {
		return nil, nil, err
	}
	if cmd.WorkflowID == "" || cmd.WorkflowVersion < 0 {
		return nil, nil, errors.New("invalid workflow submission identity")
	}
	input, err := s.normalizeInputStorage(ctx, cmd.OrganizationID, cmd.Input, false)
	if err != nil {
		return nil, nil, err
	}
	encoded, err := json.Marshal([]any{cmd.WorkflowID, cmd.WorkflowVersion, input})
	if err != nil || len(encoded) > 1048576 {
		return nil, nil, errors.New("invalid workflow submission payload")
	}
	hash := sha256.Sum256(encoded)
	intent := &domain.Submission{SubmissionKey: key, RequestDigest: hex.EncodeToString(hash[:])}
	previous, err := s.submissions.GetSubmission(ctx, key, false)
	if err != nil {
		return nil, nil, errors.New("workflow submission lookup unavailable")
	}
	if previous == nil {
		return intent, nil, nil
	}
	j, err := s.replaySubmission(ctx, *intent, *previous)
	return intent, j, err
}

func (s *Service) replaySubmission(ctx context.Context, intent, previous domain.Submission) (*domain.Job, error) {
	if intent.SubmissionKey != previous.SubmissionKey || intent.RequestDigest != previous.RequestDigest {
		return nil, domain.ErrSubmissionConflict
	}
	if previous.Validate() != nil {
		return nil, errors.New("workflow submission receipt unavailable")
	}
	j, err := s.jobRepo.Get(ctx, previous.JobID)
	if err != nil || j == nil {
		return nil, errors.New("accepted workflow submission unavailable")
	}
	if j.SubmittedBy != intent.SubmittedBy || j.OrganizationID != intent.OrganizationID || j.WorkspaceID != intent.WorkspaceID || j.Origin != "native" {
		return nil, ErrSubmitForbidden
	}
	return j, nil
}
