package job

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/executable"
)

func (s *Service) ConfigureStarts(repo domain.ExecutionStartRepo) { s.starts = repo }

// enqueueInitialStart accepts the local aggregate, not an upstream execution.
// No external engine operation may happen inside this transaction.
func (s *Service) enqueueInitialStart(ctx context.Context, j *domain.Job, steps []*domain.StepRun, compilation *wfdomain.Compilation, submission *domain.Submission) (*domain.Job, error) {
	if s.tx == nil || s.starts == nil || compilation == nil {
		return nil, errors.New("durable execution start is not configured")
	}
	var ir executable.ExecutableWorkflowIR
	if json.Unmarshal(compilation.IRJSON, &ir) != nil {
		return nil, errors.New("invalid frozen workflow compilation")
	}
	domains, err := taskDomainsForIR(ir, j.SubmittedBy)
	if err != nil {
		return nil, err
	}
	inputJSON, err := json.Marshal(map[string]any{"job_id": j.ID, "inputs": conductorInputs(j.Input)})
	if err != nil {
		return nil, errors.New("invalid frozen workflow input")
	}
	domainsJSON, err := json.Marshal(domains)
	if err != nil {
		return nil, errors.New("invalid frozen task domains")
	}
	command := domain.ExecutionStart{ID: "start_" + uuid.NewString(), JobID: j.ID, WorkflowName: compilation.ConductorName, WorkflowVersion: compilation.ConductorVersion, CompilationHash: compilation.DefinitionHash, CorrelationID: j.ID, WorkflowInput: inputJSON, TaskDomains: domainsJSON}
	command.PayloadDigest, err = command.Digest()
	if err != nil {
		return nil, err
	}
	if err := command.Validate(); err != nil {
		return nil, err
	}
	j.Origin = "native"
	accepted := j
	if err := s.tx.RunInTx(ctx, func(txCtx context.Context) error {
		if submission != nil {
			if s.submissions == nil {
				return errors.New("submission persistence unavailable")
			}
			previous, err := s.submissions.GetSubmission(txCtx, submission.SubmissionKey, true)
			if err != nil {
				return err
			}
			if previous != nil {
				accepted, err = s.replaySubmission(txCtx, *submission, *previous)
				return err
			}
		}
		if err := s.jobRepo.Save(txCtx, j, steps); err != nil {
			return err
		}
		if err := s.starts.EnqueueStart(txCtx, command); err != nil {
			return err
		}
		if submission != nil {
			copy := *submission
			copy.JobID = j.ID
			return s.submissions.SaveSubmission(txCtx, copy)
		}
		return nil
	}); err != nil {
		if errors.Is(err, domain.ErrSubmissionConflict) {
			return nil, domain.ErrSubmissionConflict
		}
		return nil, errors.New("workflow submission was not persisted")
	}
	if accepted == j {
		j.CompilationHash = command.CompilationHash
	}
	// Returning the accepted aggregate avoids a fallible read after commit.
	// The dispatcher, not this request, binds the Conductor execution later.
	return accepted, nil
}
