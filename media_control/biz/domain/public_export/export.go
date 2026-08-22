package public_export

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound      = errors.New("public export not found")
	ErrInvalidStatus = errors.New("invalid public export status")
	ErrTransition    = errors.New("invalid public export transition")
)

type Status string

const (
	StatusPending Status = "pending"
	StatusReady   Status = "ready"
	StatusFailed  Status = "failed"
	StatusRevoked Status = "revoked"
)

type Export struct {
	ID, WorkspaceID, ResourceType, ResourceID, ObjectPath, CreatedBy string
	Status                                                           Status
	CreatedAt                                                        time.Time
}
type Repository interface {
	Create(context.Context, *Export) error
	ListOwned(context.Context, string, string, string, string) ([]Export, error)
	GetOwned(context.Context, string, string) (*Export, error)
	SetStatus(context.Context, *Export, string) error
}

func Validate(rt, rid, path string) (string, string, string, error) {
	rt, rid, path = strings.TrimSpace(rt), strings.TrimSpace(rid), strings.TrimSpace(path)
	if rt == "" || rid == "" || path == "" {
		return "", "", "", errors.New("resource_type, resource_id and object_path are required")
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return "", "", "", errors.New("object_path must be a storage-relative path")
	}
	return rt, rid, path, nil
}
func ParseStatus(v string) (Status, error) {
	s := Status(strings.TrimSpace(v))
	switch s {
	case StatusPending, StatusReady, StatusFailed, StatusRevoked:
		return s, nil
	default:
		return "", ErrInvalidStatus
	}
}
func CanTransition(from, to Status) bool {
	if from == StatusPending {
		return to == StatusReady || to == StatusFailed || to == StatusRevoked
	}
	if from == StatusReady {
		return to == StatusRevoked
	}
	return false
}
