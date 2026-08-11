package agent_session

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("agent session not found")
	ErrForbidden     = errors.New("agent session does not belong to user")
	ErrTurnInFlight  = errors.New("a turn is already running")
	ErrAlreadyExists = errors.New("turn idempotency key already exists")
)

type Session struct {
	ID, UserID, WorkspaceID, AgentID, AgentType, Title, RuntimeSessionID, Status string
	ContextVersion                                                               int64
	CreatedAt, UpdatedAt                                                         time.Time
}

type Turn struct {
	ID, SessionID, IdempotencyKey, Status string
	StartedAt                             time.Time
	FinishedAt                            *time.Time
}

type Message struct {
	ID, SessionID, TurnID, Role string
	Content                     json.RawMessage
	Sequence                    int64
	CreatedAt                   time.Time
}

type Event struct {
	ID, SessionID, TurnID, Type string
	Payload                     json.RawMessage
	Sequence                    int64
	CreatedAt                   time.Time
}

type Repository interface {
	FindLatestActive(ctx context.Context, userID string) (*Session, error)
	ListOwned(ctx context.Context, userID, workspaceID string) ([]Session, error)
	CreateSession(ctx context.Context, session *Session) error
	GetOwned(ctx context.Context, sessionID, userID string) (*Session, error)
	ListMessages(ctx context.Context, sessionID string) ([]Message, error)
	ListEventsAfter(ctx context.Context, sessionID string, sequence int64) ([]Event, error)
	NextMessageSequence(ctx context.Context, sessionID string) (int64, error)
	NextEventSequence(ctx context.Context, sessionID string) (int64, error)
	CreateTurnAndMessage(ctx context.Context, turn *Turn, message *Message) error
	AppendEvent(ctx context.Context, event *Event, message *Message, runtimeSessionID string) error
	FinishTurn(ctx context.Context, turnID, status string) error
}

type TxRunner interface {
	RunInTx(ctx context.Context, fn func(context.Context) error) error
}
