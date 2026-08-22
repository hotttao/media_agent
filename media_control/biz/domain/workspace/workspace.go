package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrNotFound = errors.New("workspace not found")

type AgentType string

const (
	AgentTypeCodex      AgentType = "codex"
	AgentTypeClaudeCode AgentType = "claude-code"
)

func ParseAgentType(value string) (AgentType, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch AgentType(value) {
	case AgentTypeCodex, AgentTypeClaudeCode:
		return AgentType(value), nil
	default:
		return "", fmt.Errorf("unsupported agent type %q", value)
	}
}

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
