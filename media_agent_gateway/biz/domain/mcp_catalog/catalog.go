package mcp_catalog

import (
	"context"
	"errors"
	"time"
)

var ErrInvalid = errors.New("invalid MCP definition")
var ErrConflict = errors.New("MCP definition version conflict")
var ErrNotFound = errors.New("MCP definition not found")
var ErrForbidden = errors.New("MCP management forbidden")

type Definition struct {
	ID, ServiceName, Endpoint, Transport, AuthenticationType, Status string
	Version                                                          int64
	CreatedAt, UpdatedAt                                             time.Time
}
type Event struct {
	ID, DefinitionID string
	Version          int64
	CreatedAt        time.Time
}
type Repository interface {
	Get(context.Context, string, bool) (*Definition, error)
	Version(context.Context, string, int64) (*Definition, error)
	List(context.Context, string, int) ([]Definition, error)
	Save(context.Context, *Definition, int64) error
	Changed(context.Context, Event) error
}

type EventRepository interface {
	PendingEvents(context.Context, int) ([]Event, error)
	CompleteEvent(context.Context, string) error
	DeferEvent(context.Context, string) error
}
