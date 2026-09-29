package apitoken

import (
	"context"
	"errors"
	"time"
)

type ActorType string

const (
	StatusIssuing         Status    = "issuing"
	StatusIssuanceCleanup Status    = "issuance_cleanup"
	ActorUser             ActorType = "user"
	ActorAgent            ActorType = "agent"
	ActorPersonalWorker   ActorType = "personal_worker"
	ActorService          ActorType = "service"
)

func (t ActorType) Valid() bool {
	return t == ActorUser || t == ActorAgent || t == ActorPersonalWorker || t == ActorService
}

type Status string

const (
	StatusActive   Status = "active"
	StatusRotating Status = "rotating"
	StatusRevoking Status = "revoking"
	StatusRevoked  Status = "revoked"
	StatusExpired  Status = "expired"
)

type Token struct {
	ID, TalosKeyID, SubjectUserID, ActorID string
	ActorType                              ActorType
	OrganizationID, Name                   string
	Scopes                                 []string
	Status                                 Status
	ExpiresAt                              *time.Time
	CreatedBy                              string
	CreatedAt                              time.Time
	RotatedAt, RevokedAt                   *time.Time
	IssuanceDeadline                       *time.Time
	ClientRequestKey                       string
}

func (t Token) ActiveAt(now time.Time) bool {
	return t.Status == StatusActive && (t.ExpiresAt == nil || now.Before(*t.ExpiresAt))
}

type Repository interface {
	Create(context.Context, Token) (Token, error)
	Get(context.Context, string) (Token, error)
	GetByTalosKeyID(context.Context, string) (Token, error)
	ListBySubject(context.Context, string) ([]Token, error)
	MarkRevoking(context.Context, string, string) (Token, error)
	MarkRevoked(context.Context, string, time.Time) error
	ReplaceTalosKey(context.Context, string, string, string, *time.Time, time.Time) (Token, error)
}

var ErrNotFound = errors.New("api token not found")
var ErrConflict = errors.New("api token operation conflicts with current state")

// RevocationSource exposes durable pending work without exposing Secret data.
type RevocationSource interface {
	ListRevoking(context.Context, string, int) ([]Token, error)
}
