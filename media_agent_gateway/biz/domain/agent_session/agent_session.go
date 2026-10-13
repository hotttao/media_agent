package agent_session

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound              = errors.New("agent session not found")
	ErrForbidden             = errors.New("agent session does not belong to user")
	ErrTurnInFlight          = errors.New("a turn is already running")
	ErrAlreadyExists         = errors.New("turn idempotency key already exists")
	ErrInactive              = errors.New("agent session registration is not active")
	ErrBindingConflict       = errors.New("session binding conflict")
	ErrCredentialUnavailable = errors.New("session credential unavailable")
	ErrContextChanged        = errors.New("agent session configuration changed; create a new session")
)

const (
	ChatStatusHuman = "human"
	ChatStatusAgent = "agent"
	StatusStarting  = "starting"
	StatusActive    = "active"
	StatusFailed    = "failed"
)

type Session struct {
	ID, UserID, WorkspaceID, AgentID, AgentType, Title, Status, AgentConfigID, MetadataTargetContextVersion                        string
	EnvironmentID, ReplicaID, IncarnationID, BindingPhase, TransferID, TargetReplicaID, TargetIncarnationID, TokenHash, TokenKeyID string
	BindingVersion, ExecutionEpoch, ExpectedExecutionEpoch                                                                         int64
	ContextVersion                                                                                                                 int64
	CreatedAt, UpdatedAt                                                                                                           time.Time
}

type Repository interface {
	FindLatestUsable(ctx context.Context, userID string) (*Session, error)
	ListOwned(ctx context.Context, userID, workspaceID string) ([]Session, error)
	CreateSession(ctx context.Context, session *Session) error
	GetOwned(ctx context.Context, sessionID, userID string) (*Session, error)
	TransitionStatus(ctx context.Context, sessionID, from, to string, updatedAt time.Time) error
}

type BindingRepository interface {
	GetSession(context.Context, string) (*Session, error)
	AssignBinding(context.Context, string, int64, Binding) (*Session, error)
	BeginTransfer(context.Context, string, int64, Binding, string) (*Session, error)
	RetargetTransfer(context.Context, string, int64, string, Binding) (*Session, error)
	CompleteTransfer(context.Context, string, int64, string, Binding) (*Session, error)
	EnsureCredential(context.Context, string, string, string) (*Session, error)
	EnsureIdentity(context.Context, string, string) (*Session, error)
	BoundSessions(context.Context, string, int) ([]Session, error)
	PendingTransfers(context.Context, int) ([]Session, error)
}
type Binding struct {
	EnvironmentID, ReplicaID, IncarnationID string
	ExecutionEpoch                          int64
}
type CredentialIssuer interface {
	Issue(*Session) (string, string, string, error)
	Reconstruct(*Session) (string, error)
}

type CredentialRepository interface {
	ByTokenHash(context.Context, string) (*Session, error)
}
