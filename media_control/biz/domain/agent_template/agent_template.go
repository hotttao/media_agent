package agent_template

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("agent template not found")
	ErrConflict = errors.New("agent template version conflict")
	ErrArchived = errors.New("agent template is archived")
)

type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

type AgentTemplate struct {
	ID, WorkspaceID, OwnerUserID, Name, ConfigurationJSON string
	Status                                                Status
	Version                                               int64
	CreatedAt, UpdatedAt                                  time.Time
}

type Repository interface {
	Create(context.Context, *AgentTemplate) error
	ListOwned(context.Context, string, string) ([]AgentTemplate, error)
	GetOwned(context.Context, string, string) (*AgentTemplate, error)
	Update(context.Context, *AgentTemplate, int64) error
	Archive(context.Context, *AgentTemplate, int64) error
}

func Validate(name, configurationJSON string) (string, string, error) {
	name = strings.TrimSpace(name)
	configurationJSON = strings.TrimSpace(configurationJSON)
	if name == "" {
		return "", "", errors.New("agent template name is required")
	}
	if configurationJSON == "" || !json.Valid([]byte(configurationJSON)) {
		return "", "", errors.New("configuration_json must be valid JSON")
	}
	return name, configurationJSON, nil
}
