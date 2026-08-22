package runtime_environment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNotFound          = errors.New("runtime environment not found")
	ErrPlacementNotFound = errors.New("workspace placement not found")
	ErrTokenNotFound     = errors.New("environment token not found")
	ErrConflict          = errors.New("runtime environment version conflict")
)

type EnvironmentStatus string

const (
	StatusActive   EnvironmentStatus = "active"
	StatusDraining EnvironmentStatus = "draining"
	StatusOffline  EnvironmentStatus = "offline"
)

type PlacementStatus string

const (
	PlacementPending  PlacementStatus = "pending"
	PlacementPlaced   PlacementStatus = "placed"
	PlacementFailed   PlacementStatus = "failed"
	PlacementUnplaced PlacementStatus = "unplaced"
)

type Environment struct {
	ID, Name, Endpoint, CapacityJSON string
	Status                           EnvironmentStatus
	Version                          int64
	CreatedAt, UpdatedAt             time.Time
}
type Placement struct {
	WorkspaceID, EnvironmentID, Reason, VolumeRef string
	Status                                        PlacementStatus
	Version                                       int64
	UpdatedAt                                     time.Time
}
type Token struct {
	ID, EnvironmentID, Name, TokenHash, CreatedBy string
	CreatedAt                                     time.Time
	RevokedAt                                     *time.Time
}
type EnvironmentPrincipal struct{ EnvironmentID string }

type Repository interface {
	CreateEnvironment(context.Context, *Environment) error
	ListEnvironments(context.Context) ([]Environment, error)
	GetEnvironment(context.Context, string) (*Environment, error)
	UpdateEnvironment(context.Context, *Environment, int64) error
	DeleteEnvironment(context.Context, *Environment, int64) error
	FindPlacement(context.Context, string) (*Placement, error)
	UpsertPlacement(context.Context, *Placement, int64) error
	CreateToken(context.Context, *Token) error
	ListActiveTokens(context.Context, string) ([]Token, error)
	GetToken(context.Context, string, string) (*Token, error)
	RevokeToken(context.Context, *Token) error
	FindActiveTokenByHash(context.Context, string) (*EnvironmentPrincipal, error)
}

func ValidateToken(name, createdBy string) (string, string, error) {
	name, createdBy = strings.TrimSpace(name), strings.TrimSpace(createdBy)
	if name == "" || createdBy == "" {
		return "", "", errors.New("name and created_by are required")
	}
	if len(name) > 128 {
		return "", "", errors.New("token name must not exceed 128 characters")
	}
	return name, createdBy, nil
}

func Validate(name, endpoint, capacity string) (string, string, string, error) {
	name, endpoint = strings.TrimSpace(name), strings.TrimSpace(endpoint)
	capacity = strings.TrimSpace(capacity)
	if name == "" || endpoint == "" {
		return "", "", "", errors.New("name and endpoint are required")
	}
	if capacity == "" {
		capacity = "{}"
	}
	var value any
	if err := json.Unmarshal([]byte(capacity), &value); err != nil {
		return "", "", "", fmt.Errorf("capacity_json must be valid JSON: %w", err)
	}
	if _, ok := value.(map[string]any); !ok {
		return "", "", "", errors.New("capacity_json must be a JSON object")
	}
	return name, endpoint, capacity, nil
}

func ParseStatus(value string) (EnvironmentStatus, error) {
	s := EnvironmentStatus(strings.TrimSpace(value))
	switch s {
	case StatusActive, StatusDraining, StatusOffline:
		return s, nil
	default:
		return "", fmt.Errorf("invalid environment status %q", value)
	}
}
