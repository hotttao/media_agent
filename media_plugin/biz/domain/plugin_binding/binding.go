package plugin_binding

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid MCP binding")
	ErrForbidden   = errors.New("MCP binding permission denied")
	ErrNotFound    = errors.New("MCP binding not found")
	ErrConflict    = errors.New("MCP binding version conflict")
	ErrUnavailable = errors.New("MCP binding service unavailable")
	ErrReauth      = errors.New("MCP reconnect required")
)

type Binding struct {
	ID, WorkspaceID, DefinitionID                                  string
	DefinitionVersion, Version                                     int64
	Enabled                                                        bool
	AllowedMethods, AllowedTools, AllowedPrompts, AllowedResources []string
	CredentialID, CredentialHeader, UpdatedBy, Checksum            string
	CreatedAt, UpdatedAt                                           time.Time
}
type Credential struct {
	ID, BindingID string
	Ciphertext    []byte
}
type Repository interface {
	Get(context.Context, string, bool) (*Binding, error)
	Version(context.Context, string, int64) (*Binding, error)
	List(context.Context, string) ([]Binding, error)
	Versions(context.Context, string) ([]Binding, error)
	Save(context.Context, *Binding, int64, *Credential) error
	Credential(context.Context, string, string) ([]byte, error)
}
