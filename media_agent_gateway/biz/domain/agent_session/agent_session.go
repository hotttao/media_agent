package agent_session

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("agent session not found")
	ErrForbidden     = errors.New("agent session does not belong to user")
	ErrTurnInFlight  = errors.New("a turn is already running")
	ErrAlreadyExists = errors.New("turn idempotency key already exists")
	ErrInactive      = errors.New("agent session registration is not active")
)

const (
	ChatStatusHuman = "human"
	ChatStatusAgent = "agent"
	StatusStarting  = "starting"
	StatusActive    = "active"
	StatusFailed    = "failed"
)

type Session struct {
	ID, UserID, WorkspaceID, AgentID, AgentType, Title, Status string
	ContextVersion                                             int64
	CreatedAt, UpdatedAt                                       time.Time
}

type Repository interface {
	FindLatestUsable(ctx context.Context, userID string) (*Session, error)
	ListOwned(ctx context.Context, userID, workspaceID string) ([]Session, error)
	CreateSession(ctx context.Context, session *Session) error
	GetOwned(ctx context.Context, sessionID, userID string) (*Session, error)
	TransitionStatus(ctx context.Context, sessionID, from, to string, updatedAt time.Time) error
}
