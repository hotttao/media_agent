// Package start_delivery launches frozen native Jobs outside database
// transactions. It never reissues a command whose upstream outcome is unknown.
package start_delivery

import (
	"context"
	"encoding/json"
	"errors"

	domain "media_agent/media_workflow/biz/domain/job"
)

var ErrTemporarilyUnavailable = errors.New("execution start delivery temporarily unavailable")

type Flow struct {
	Starts domain.ExecutionStartRepo
	Jobs   domain.JobRepo
	Engine domain.ExecutionStartEngine
}

func (f *Flow) DispatchOne(ctx context.Context) error {
	if f.Starts == nil || f.Jobs == nil || f.Engine == nil {
		return errors.New("execution start delivery is not configured")
	}
	command, err := f.Starts.ClaimStart(ctx)
	if err != nil || command == nil {
		return err
	}
	release := func(code string) error {
		if err := f.Starts.ReleaseStartClaim(ctx, *command, code); err != nil {
			return err
		}
		return ErrTemporarilyUnavailable
	}
	if command.Validate() != nil {
		return release("invalid_frozen_start")
	}
	j, err := f.Jobs.Get(ctx, command.JobID)
	if err != nil || j == nil {
		return release("job_unavailable")
	}
	if j.Status == domain.JobCancelled && j.Origin == "native" && j.CompilationHash == command.CompilationHash {
		if repo, ok := f.Starts.(domain.ExecutionStartCancellationRepo); ok {
			if engine, ok := f.Engine.(domain.ExecutionStartCleanupEngine); ok {
				return f.cleanupCancelled(ctx, *command, repo, engine, release)
			}
		}
	}
	// Other terminal or mismatched Jobs must not be bound or revived. A queued
	// command observed after cancellation must never authorize another POST.
	if j.Status != domain.JobPending || j.Origin != "native" || j.CompilationHash != command.CompilationHash || j.ConductorWorkflowID != "" {
		return release("job_no_longer_pending")
	}
	var executionID string
	switch command.Status {
	case "queued":
		var input map[string]any
		var domains map[string]string
		if json.Unmarshal(command.WorkflowInput, &input) != nil || json.Unmarshal(command.TaskDomains, &domains) != nil {
			return release("invalid_frozen_start")
		}
		// A stale lease or concurrent cancellation must prevent the POST.
		if err := f.Starts.MarkStartSent(ctx, *command); err != nil {
			return release("start_mark_unavailable")
		}
		executionID, err = f.Engine.Start(ctx, command.WorkflowName, command.WorkflowVersion, command.CorrelationID, input, domains)
	case "uncertain":
		// Read only: even a definitive-looking empty search is not proof that
		// the previous POST did not commit or is not still in flight.
		executionID, err = f.Engine.FindStartedExecution(ctx, command.WorkflowName, command.CorrelationID)
	default:
		return release("invalid_start_state")
	}
	if err != nil || executionID == "" {
		return release("start_outcome_unconfirmed")
	}
	if err := f.Starts.FinishStart(ctx, *command, executionID); err != nil {
		// A committed upstream start with failed local binding stays uncertain;
		// the next lease reconciles by correlation rather than launching again.
		return release("start_binding_unavailable")
	}
	return nil
}

func (f *Flow) cleanupCancelled(ctx context.Context, command domain.ExecutionStart, repo domain.ExecutionStartCancellationRepo, engine domain.ExecutionStartCleanupEngine, release func(string) error) error {
	id := command.ConductorWorkflowID
	if command.Status == "uncertain" {
		var err error
		id, err = engine.FindStartedExecution(ctx, command.WorkflowName, command.CorrelationID)
		if err != nil || id == "" {
			return release("cancel_execution_unconfirmed")
		}
	} else if command.Status != "cancel_pending" || id == "" {
		return release("invalid_cancel_state")
	}
	execution, err := engine.Get(ctx, id)
	if err != nil || execution == nil {
		return release("cancel_execution_unavailable")
	}
	terminal := func(status string) bool {
		switch status {
		case "COMPLETED", "FAILED", "TIMED_OUT", "TERMINATED", "FAILED_WITH_TERMINAL_ERROR":
			return true
		}
		return false
	}
	if !terminal(execution.Status) {
		if err := engine.Terminate(ctx, id, "cancelled by user"); err != nil {
			return release("cancel_termination_unconfirmed")
		}
		execution, err = engine.Get(ctx, id)
		if err != nil || execution == nil || !terminal(execution.Status) {
			return release("cancel_termination_unconfirmed")
		}
	}
	if err := repo.FinishCancelledStart(ctx, command, id); err != nil {
		return release("cancel_binding_unavailable")
	}
	return nil
}
