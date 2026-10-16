package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrNotFound = errors.New("workspace not found")
var ErrConflict = errors.New("workspace configuration version conflict")

type AgentType string

const (
	AgentTypeCodex      AgentType = "codex"
	AgentTypeGoose      AgentType = "goose"
	AgentTypeClaudeCode AgentType = "claude-code"
)

func ParseAgentType(value string) (AgentType, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch AgentType(value) {
	case AgentTypeCodex, AgentTypeClaudeCode, AgentTypeGoose:
		return AgentType(value), nil
	default:
		return "", fmt.Errorf("unsupported agent type %q", value)
	}
}

type Workspace struct {
	ID, OrganizationID, TeamID, CreatedBy, OwnerUserID, Name, DesiredState string
	Version                                                                int64
	CreatedAt, UpdatedAt                                                   time.Time
}

type AgentConfiguration struct {
	ID, WorkspaceID, AgentID, AgentType, ModelConnectionID string
	EncryptedAPIKey                                        []byte
	Version                                                int64
	CreatedAt, UpdatedAt                                   time.Time
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

// AccessRepository adds organization-scoped reads used by the Workspace API.
// Legacy owner-scoped methods remain on Repository for callers that have not
// yet migrated their own authorization boundary.
type AccessRepository interface {
	Repository
	ListByTeams(context.Context, []string) ([]ConfiguredWorkspace, error)
	Get(context.Context, string) (*ConfiguredWorkspace, error)
}

type TeamAuthorization struct {
	Allowed           bool
	OrganizationID    string
	TeamID            string
	TeamRole          string
	OrganizationRole  string
	EffectiveFeatures []string
}

type TeamAccess interface {
	ListTeams(context.Context, string) ([]string, error)
	Authorize(context.Context, string, string, string) (TeamAuthorization, error)
}
