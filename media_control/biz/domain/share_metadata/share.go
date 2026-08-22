package share_metadata

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("share metadata not found")
	ErrRevoked  = errors.New("share metadata is revoked")
)

type Status string

const (
	StatusActive  Status = "active"
	StatusRevoked Status = "revoked"
)

type Share struct {
	ID, WorkspaceID, ResourceType, ResourceID, OwnerUserID string
	Status                                                 Status
	CreatedAt, RevokedAt                                   *time.Time
}
type Repository interface {
	Create(context.Context, *Share) error
	ListOwned(context.Context, string, string, string, string) ([]Share, error)
	GetOwned(context.Context, string, string) (*Share, error)
	Revoke(context.Context, *Share) error
}

func Validate(resourceType, resourceID string) (string, string, error) {
	resourceType, resourceID = strings.TrimSpace(resourceType), strings.TrimSpace(resourceID)
	if resourceType == "" || resourceID == "" {
		return "", "", errors.New("resource_type and resource_id are required")
	}
	return resourceType, resourceID, nil
}
