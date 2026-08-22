package skill

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"
)

var (
	ErrNotFound      = errors.New("skill not found")
	ErrConflict      = errors.New("skill version conflict")
	ErrArchived      = errors.New("skill is archived")
	ErrInvalidStatus = errors.New("invalid skill status")
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
	StatusArchived Status = "archived"
)

type Skill struct {
	ID, WorkspaceID, OwnerUserID, Name, ContentPath, Checksum string
	Status                                                    Status
	Version                                                   int64
	CreatedAt, UpdatedAt                                      time.Time
}

type Repository interface {
	Create(context.Context, *Skill) error
	ListOwned(context.Context, string, string) ([]Skill, error)
	GetOwned(context.Context, string, string) (*Skill, error)
	Update(context.Context, *Skill, int64) error
	SetStatus(context.Context, *Skill, int64) error
	Archive(context.Context, *Skill, int64) error
}

func Validate(name, contentPath, checksum string) (string, string, string, error) {
	name, contentPath, checksum = strings.TrimSpace(name), strings.TrimSpace(contentPath), strings.TrimSpace(checksum)
	if name == "" {
		return "", "", "", errors.New("skill name is required")
	}
	if contentPath == "" || path.IsAbs(contentPath) || contentPath == "." || strings.HasPrefix(path.Clean(contentPath), "../") || path.Clean(contentPath) == ".." {
		return "", "", "", errors.New("content_path must be a relative path inside storage")
	}
	if checksum == "" {
		return "", "", "", errors.New("skill checksum is required")
	}
	return name, path.Clean(contentPath), checksum, nil
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
