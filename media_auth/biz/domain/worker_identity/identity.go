package workeridentity

import (
	"context"
	"errors"
	"time"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
	StatusDeleted  Status = "deleted"
)

func (s Status) CanTransitionTo(next Status) bool {
	switch s {
	case StatusActive:
		return next == StatusActive || next == StatusDisabled || next == StatusDeleted
	case StatusDisabled:
		return next == StatusDisabled || next == StatusDeleted
	case StatusDeleted:
		return next == StatusDeleted
	}
	return false
}

type Identity struct {
	ID, OwnerUserID, OrganizationID, Name string
	Status                                Status
	CreatedAt                             time.Time
	DisabledAt, DeletedAt                 *time.Time
}

func (i Identity) ActorID() string { return "Worker:" + i.ID }

var (
	ErrNotFound  = errors.New("worker identity not found")
	ErrConflict  = errors.New("worker identity changed concurrently")
	ErrInvalid   = errors.New("invalid worker identity request")
	ErrForbidden = errors.New("worker identity operation forbidden")
)

type Repository interface {
	Create(context.Context, Identity) (Identity, error)
	Get(context.Context, string) (Identity, error)
	ListByOwner(context.Context, string) ([]Identity, error)
	UpdateStatus(context.Context, string, string, Status, Status, time.Time) error
}

type InactiveSource interface {
	ListInactive(context.Context, string, int) ([]Identity, error)
}
