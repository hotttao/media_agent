package workspace

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("workspace not found")

type Workspace struct {
	ID, OwnerUserID, Name, DesiredState string
	Version                             int64
	CreatedAt, UpdatedAt                time.Time
}

type AgentConfiguration struct {
	ID, WorkspaceID, AgentID, AgentType string
	EncryptedAPIKey                     []byte
	Version                             int64
	CreatedAt, UpdatedAt                time.Time
}

type ConfiguredWorkspace struct {
	Workspace
	AgentConfiguration
}

type Repository interface {
	Create(context.Context, *Workspace, *AgentConfiguration) error
	ListOwned(context.Context, string) ([]ConfiguredWorkspace, error)
	GetOwned(context.Context, string, string) (*ConfiguredWorkspace, error)
	Update(context.Context, *Workspace, *AgentConfiguration) error
}
