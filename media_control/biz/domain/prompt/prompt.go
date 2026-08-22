package prompt

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("prompt not found")
	ErrConflict = errors.New("prompt version conflict")
	ErrArchived = errors.New("prompt is archived")
)

type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

type Prompt struct {
	ID, WorkspaceID, OwnerUserID, Name, Content string
	Status                                      Status
	Version                                     int64
	CreatedAt, UpdatedAt                        time.Time
}

type Repository interface {
	Create(context.Context, *Prompt) error
	ListOwned(context.Context, string, string) ([]Prompt, error)
	GetOwned(context.Context, string, string) (*Prompt, error)
	Update(context.Context, *Prompt, int64) error
	Archive(context.Context, *Prompt, int64) error
}

func Validate(name, content string) (string, string, error) {
	name = strings.TrimSpace(name)
	content = strings.TrimSpace(content)
	if name == "" {
		return "", "", errors.New("prompt name is required")
	}
	if content == "" {
		return "", "", errors.New("prompt content is required")
	}
	return name, content, nil
}
