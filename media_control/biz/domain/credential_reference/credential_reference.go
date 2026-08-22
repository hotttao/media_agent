package credential_reference

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("credential reference not found")
	ErrConflict = errors.New("credential reference version conflict")
	ErrRevoked  = errors.New("credential reference is revoked")
)

type Status string

const (
	StatusActive  Status = "active"
	StatusRevoked Status = "revoked"
)

type CredentialReference struct {
	ID, WorkspaceID, OwnerUserID, Name, ProviderID string
	Status                                         Status
	Version                                        int64
	CreatedAt, UpdatedAt                           time.Time
}
type Repository interface {
	Create(context.Context, *CredentialReference, string) error
	ListOwned(context.Context, string, string) ([]CredentialReference, error)
	GetOwned(context.Context, string, string) (*CredentialReference, error)
	Update(context.Context, *CredentialReference, string, int64) error
	Revoke(context.Context, *CredentialReference, int64) error
}

func Validate(name, providerID, secretRef string) (string, string, string, error) {
	name, providerID, secretRef = strings.TrimSpace(name), strings.TrimSpace(providerID), strings.TrimSpace(secretRef)
	if name == "" || providerID == "" || secretRef == "" {
		return "", "", "", errors.New("name, provider_id and secret_ref are required")
	}
	if strings.ContainsAny(secretRef, "\r\n") {
		return "", "", "", errors.New("secret_ref must be a single external reference")
	}
	return name, providerID, secretRef, nil
}
