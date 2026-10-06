package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

var ErrStartConflict = errors.New("execution start command conflicts with current state")

// ExecutionStart freezes one native Job's initial launch. Uncertain means a
// Start may have been sent, and must never be blindly changed back to queued.
type ExecutionStart struct {
	ID, JobID, WorkflowName, CompilationHash, CorrelationID string
	WorkflowVersion                                         int
	WorkflowInput, TaskDomains                              json.RawMessage
	PayloadDigest, Status, ClaimToken, ConductorWorkflowID  string
}

func (s ExecutionStart) Digest() (string, error) {
	var input, domains any
	if json.Unmarshal(s.WorkflowInput, &input) != nil || json.Unmarshal(s.TaskDomains, &domains) != nil {
		return "", errors.New("invalid frozen start JSON")
	}
	encoded, err := json.Marshal([]any{s.JobID, s.WorkflowName, s.WorkflowVersion, s.CompilationHash, s.CorrelationID, input, domains})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func (s ExecutionStart) Validate() error {
	if s.ID == "" || s.JobID == "" || s.WorkflowName == "" || s.WorkflowVersion < 1 || s.CorrelationID != s.JobID || len(s.CompilationHash) != 64 || len(s.WorkflowInput) > 1048576 || len(s.TaskDomains) > 1048576 {
		return errors.New("invalid frozen start identity or size")
	}
	if _, err := hex.DecodeString(s.CompilationHash); err != nil || strings.ToLower(s.CompilationHash) != s.CompilationHash {
		return errors.New("invalid compilation hash")
	}
	var input map[string]any
	var domains map[string]string
	if json.Unmarshal(s.WorkflowInput, &input) != nil || input["job_id"] != s.JobID || json.Unmarshal(s.TaskDomains, &domains) != nil || domains == nil {
		return errors.New("invalid frozen start payload")
	}
	for task, domain := range domains {
		if task == "" || domain == "" {
			return errors.New("invalid frozen task domain")
		}
	}
	digest, err := s.Digest()
	if err != nil || digest != s.PayloadDigest {
		return errors.New("frozen start digest mismatch")
	}
	return nil
}

type ExecutionStartRepo interface {
	// EnqueueStart requires the same caller transaction as Job + StepRun Save.
	EnqueueStart(context.Context, ExecutionStart) error
	ClaimStart(context.Context) (*ExecutionStart, error)
	// MarkStartSent durably closes the queued state before any external call.
	MarkStartSent(context.Context, ExecutionStart) error
	FinishStart(context.Context, ExecutionStart, string) error
	ReleaseStartClaim(context.Context, ExecutionStart, string) error
}

// ExecutionStartEngine separates read-only reconciliation from launching. An
// absent correlation match never authorizes resending an uncertain command.
type ExecutionStartEngine interface {
	Start(context.Context, string, int, string, map[string]any, map[string]string) (string, error)
	FindStartedExecution(context.Context, ExecutionStart) (string, error)
}

type ExecutionStartCancellationRepo interface {
	// false means no initial start command exists; no Job mutation is made.
	CancelInitialStart(context.Context, string) (bool, error)
	FinishCancelledStart(context.Context, ExecutionStart, string) error
}

type ExecutionStartCleanupEngine interface {
	ExecutionStartEngine
	Get(context.Context, string) (*EngineExecution, error)
	Terminate(context.Context, string, string) error
}
