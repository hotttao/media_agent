package memory_store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound      = errors.New("memory store not found")
	ErrConflict      = errors.New("memory store version conflict")
	ErrInvalidStatus = errors.New("invalid memory store status")
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

type MemoryStore struct {
	ID, WorkspaceID, OwnerUserID, Name, StoreType, ConfigurationJSON, CredentialReferenceID string
	Status                                                                                  Status
	Version                                                                                 int64
	CreatedAt, UpdatedAt                                                                    time.Time
}
type Repository interface {
	Create(context.Context, *MemoryStore) error
	ListOwned(context.Context, string, string) ([]MemoryStore, error)
	GetOwned(context.Context, string, string) (*MemoryStore, error)
	Update(context.Context, *MemoryStore, int64) error
	SetStatus(context.Context, *MemoryStore, int64) error
}

func Validate(name, storeType, configuration, credentialID string) (string, string, string, string, error) {
	name, storeType, configuration, credentialID = strings.TrimSpace(name), strings.TrimSpace(storeType), strings.TrimSpace(configuration), strings.TrimSpace(credentialID)
	if name == "" || storeType == "" {
		return "", "", "", "", errors.New("name and store_type are required")
	}
	if configuration == "" || !json.Valid([]byte(configuration)) {
		return "", "", "", "", errors.New("configuration_json must be valid JSON")
	}
	return name, storeType, configuration, credentialID, nil
}
func ParseStatus(value string) (Status, error) {
	switch Status(strings.ToLower(strings.TrimSpace(value))) {
	case StatusActive:
		return StatusActive, nil
	case StatusDisabled:
		return StatusDisabled, nil
	default:
		return "", ErrInvalidStatus
	}
}
