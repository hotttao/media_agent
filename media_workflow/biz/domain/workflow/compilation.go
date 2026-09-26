package workflow

import (
	"context"
	"time"
)

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
	Save(ctx context.Context, compilation *Compilation) error
	MarkRegistered(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id, message string) error
	GetRegistered(ctx context.Context, workflowID string, workflowVersion int) (*Compilation, error)
}

type Publisher interface {
	CompileAndRegister(ctx context.Context, workflow *Workflow) error
}
