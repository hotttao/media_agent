package workflow

import (
	"context"
	"errors"
	"time"
)

var ErrCompilationConflict = errors.New("workflow version already has a different frozen compilation")

type Compilation struct {
	ID               string
	WorkflowID       string
	WorkflowVersion  int
	ConductorName    string
	ConductorVersion int
	IRJSON           []byte
	DefinitionJSON   []byte
	DefinitionHash   string
	Status           string
	ErrorMessage     string
	CreatedAt        time.Time
	RegisteredAt     *time.Time
}

type CompilationRepo interface {
	// Save inserts once or replays the identical frozen payload. It never
	// overwrites a version's identity, IR, definition or registration state.
	Save(ctx context.Context, compilation *Compilation) error
	MarkRegistered(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id, message string) error
	GetRegistered(ctx context.Context, workflowID string, workflowVersion int) (*Compilation, error)
}

type Publisher interface {
	AcceptPublication(ctx context.Context, workflow *Workflow, callerID string) (*Publication, error)
}
