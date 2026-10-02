package job

import (
	"context"
	"encoding/json"
	"errors"
)

var ErrResumeConflict = errors.New("historical Job already resumed or is not awaiting resume")

// ResumeStart is the durable command to register and start one recovered
// Conductor execution. The frozen payload is replayed verbatim after a crash.
type ResumeStart struct {
	ID                  string
	JobID               string
	Generation          int
	IdempotencyKey      string
	CorrelationID       string
	DefinitionJSON      json.RawMessage
	TaskDomainsJSON     json.RawMessage
	WorkflowInputJSON   json.RawMessage
	ConductorWorkflowID string
}

type ResumeRepo interface {
	// BeginResume atomically freezes the patched Job input and enqueues Start.
	// created=false means the exact idempotency key was already accepted.
	BeginResume(ctx context.Context, start ResumeStart, input JobInput) (created bool, err error)
	ClaimPendingResume(ctx context.Context) (*ResumeStart, error)
	MarkResumeStarted(ctx context.Context, start ResumeStart, executionID, definitionHash string) error
	ReleaseResumeClaim(ctx context.Context, startID, reason string) error
}
