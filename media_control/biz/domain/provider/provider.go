package provider

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound      = errors.New("provider not found")
	ErrConflict      = errors.New("provider version conflict")
	ErrInvalidStatus = errors.New("invalid provider status")
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

type Provider struct {
	ID, Name, ProviderType, Endpoint, CapabilitiesJSON string
	Status                                             Status
	Version                                            int64
	CreatedAt, UpdatedAt                               time.Time
}
type Repository interface {
	Create(context.Context, *Provider) error
	List(context.Context) ([]Provider, error)
	Get(context.Context, string) (*Provider, error)
	Update(context.Context, *Provider, int64) error
	SetStatus(context.Context, *Provider, int64) error
}

func Validate(name, providerType, endpoint, capabilities string) (string, string, string, string, error) {
	name, providerType, endpoint, capabilities = strings.TrimSpace(name), strings.TrimSpace(providerType), strings.TrimSpace(endpoint), strings.TrimSpace(capabilities)
	if name == "" || providerType == "" || endpoint == "" {
		return "", "", "", "", errors.New("provider name, provider_type and endpoint are required")
	}
	if capabilities == "" || !json.Valid([]byte(capabilities)) {
		return "", "", "", "", errors.New("capabilities_json must be valid JSON")
	}
	return name, providerType, endpoint, capabilities, nil
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
