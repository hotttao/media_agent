package workspace_metadata

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound            = errors.New("tag not found")
	ErrConflict            = errors.New("tag version conflict")
	ErrAssociationNotFound = errors.New("resource tag not found")
)

type Tag struct {
	ID, WorkspaceID, Name, Color, CreatedBy string
	Version                                 int64
	CreatedAt, UpdatedAt                    time.Time
}
type ResourceTag struct {
	TagID, WorkspaceID, ResourceType, ResourceID string
	CreatedAt                                    time.Time
}
type Repository interface {
	Create(context.Context, *Tag) error
	ListOwned(context.Context, string, string) ([]Tag, error)
	GetOwned(context.Context, string, string) (*Tag, error)
	Update(context.Context, *Tag, int64) error
	Delete(context.Context, *Tag, int64) error
	Attach(context.Context, *ResourceTag) error
	Detach(context.Context, *ResourceTag) error
	ListResourceTags(context.Context, string, string, string, string) ([]Tag, error)
}

func Validate(name, color string) (string, string, error) {
	name = strings.TrimSpace(name)
	color = strings.TrimSpace(color)
	if name == "" {
		return "", "", errors.New("tag name is required")
	}
	if len(name) > 128 {
		return "", "", errors.New("tag name is too long")
	}
	return name, color, nil
}
func ValidateResource(resourceType, resourceID string) (string, string, error) {
	resourceType, resourceID = strings.TrimSpace(resourceType), strings.TrimSpace(resourceID)
	if resourceType == "" || resourceID == "" {
		return "", "", errors.New("resource_type and resource_id are required")
	}
	return resourceType, resourceID, nil
}
