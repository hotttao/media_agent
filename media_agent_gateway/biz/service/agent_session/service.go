package agent_session

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
)

type AgentContext struct {
	WorkspaceID, AgentID, AgentType, APIKey string
	ContextVersion                          int64
}
type ContextProvider interface {
	Default(context.Context, string) (AgentContext, error)
	Resolve(context.Context, string, string, string) (AgentContext, error)
}

func (s *Service) Create(ctx context.Context, userID, workspaceID, title, authToken string) (*domain.Session, error) {
	config, err := s.contexts.Resolve(ctx, userID, workspaceID, authToken)
	if err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "New session"
	}
	now := time.Now().UTC()
	created := &domain.Session{ID: uuid.NewString(), UserID: userID, WorkspaceID: config.WorkspaceID, AgentID: config.AgentID, AgentType: config.AgentType, Title: title, ContextVersion: config.ContextVersion, Status: "active", ChatStatus: domain.ChatStatusHuman, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateSession(ctx, created); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Service) List(ctx context.Context, userID, workspaceID string) ([]domain.Session, error) {
	return s.repo.ListOwned(ctx, userID, workspaceID)
}

func (s *Service) RuntimeContext(ctx context.Context, session *domain.Session, authToken string) (AgentContext, error) {
	return s.contexts.Resolve(ctx, session.UserID, session.WorkspaceID, authToken)
}

type Service struct {
	repo     domain.Repository
	tx       domain.TxRunner
	contexts ContextProvider
}

func New(repo domain.Repository, tx domain.TxRunner, contexts ContextProvider) *Service {
	return &Service{repo: repo, tx: tx, contexts: contexts}
}

func (s *Service) DefaultSession(ctx context.Context, userID string) (*domain.Session, error) {
	found, err := s.repo.FindLatestActive(ctx, userID)
	if err == nil {
		return found, nil
	}
	if err != domain.ErrNotFound {
		return nil, err
	}
	config, err := s.contexts.Default(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	created := &domain.Session{ID: uuid.NewString(), UserID: userID, WorkspaceID: config.WorkspaceID, AgentID: config.AgentID, AgentType: config.AgentType, Title: "New session", ContextVersion: config.ContextVersion, Status: "active", ChatStatus: domain.ChatStatusHuman, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateSession(ctx, created); err != nil {
		// A concurrent request may have created the user's default session.
		if existing, findErr := s.repo.FindLatestActive(ctx, userID); findErr == nil {
			return existing, nil
		}
		return nil, err
	}
	return created, nil
}

func (s *Service) Messages(ctx context.Context, sessionID, userID string) ([]domain.Message, error) {
	if _, err := s.repo.GetOwned(ctx, sessionID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListMessages(ctx, sessionID)
}

func (s *Service) EventsAfter(ctx context.Context, sessionID, userID string, sequence int64) ([]domain.Event, error) {
	if _, err := s.repo.GetOwned(ctx, sessionID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListEventsAfter(ctx, sessionID, sequence)
}

func (s *Service) RunningTurn(ctx context.Context, sessionID, userID string) (*domain.Session, *domain.Turn, error) {
	session, err := s.repo.GetOwned(ctx, sessionID, userID)
	if err != nil {
		return nil, nil, err
	}
	turn, err := s.repo.GetRunningTurn(ctx, sessionID)
	if err != nil {
		return session, nil, err
	}
	return session, turn, nil
}

func (s *Service) StartTurn(ctx context.Context, sessionID, userID, key, text string) (*domain.Session, *domain.Turn, error) {
	session, err := s.repo.GetOwned(ctx, sessionID, userID)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(key) == "" {
		key = uuid.NewString()
	}
	turn := &domain.Turn{ID: uuid.NewString(), SessionID: sessionID, IdempotencyKey: key, Status: "running", StartedAt: time.Now().UTC()}
	err = s.tx.RunInTx(ctx, func(txCtx context.Context) error {
		seq, err := s.repo.NextMessageSequence(txCtx, sessionID)
		if err != nil {
			return err
		}
		content, _ := json.Marshal([]map[string]string{{"type": "text", "text": text}})
		msg := &domain.Message{ID: uuid.NewString(), SessionID: sessionID, TurnID: turn.ID, Role: "user", Content: content, Sequence: seq, CreatedAt: time.Now().UTC()}
		return s.repo.CreateTurnAndMessage(txCtx, turn, msg)
	})
	return session, turn, err
}

func (s *Service) PersistEvent(ctx context.Context, sessionID, turnID string, raw json.RawMessage) (*domain.Event, error) {
	runtimeEvent, err := domain.ParseRuntimeEvent(raw)
	if err != nil {
		return nil, err
	}
	event := &domain.Event{ID: uuid.NewString(), SessionID: sessionID, TurnID: turnID, Type: runtimeEvent.Type, Payload: raw, CreatedAt: time.Now().UTC()}
	err = s.tx.RunInTx(ctx, func(txCtx context.Context) error {
		sequence, err := s.repo.NextEventSequence(txCtx, sessionID)
		if err != nil {
			return err
		}
		if runtimeEvent.IsToolCall() && runtimeEvent.ItemID != "" {
			existing, findErr := s.repo.FindCompletedItemEvent(txCtx, turnID, runtimeEvent.ItemID, runtimeEvent.ItemKind)
			if findErr == nil {
				event = existing
				return nil
			}
			if !errors.Is(findErr, domain.ErrNotFound) {
				return findErr
			}
		}
		event.Sequence = sequence
		var message *domain.Message
		if runtimeEvent.CreatesMessage() {
			var envelope struct {
				Item json.RawMessage `json:"item"`
			}
			if err := json.Unmarshal(raw, &envelope); err != nil {
				return err
			}
			var item struct {
				Role any `json:"role"`
				Kind any `json:"kind"`
			}
			if err := json.Unmarshal(envelope.Item, &item); err != nil {
				return err
			}
			role := normalizeRole(item.Role, item.Kind)
			seq, err := s.repo.NextMessageSequence(txCtx, sessionID)
			if err != nil {
				return err
			}
			message = &domain.Message{ID: uuid.NewString(), SessionID: sessionID, TurnID: turnID, Role: role, Content: envelope.Item, Sequence: seq, CreatedAt: time.Now().UTC()}
		}
		return s.repo.AppendEvent(txCtx, event, message, runtimeEvent.SessionID)
	})
	if err != nil {
		return nil, err
	}
	return event, nil
}

func (s *Service) FinishTurn(ctx context.Context, turnID, status string) error {
	return s.tx.RunInTx(ctx, func(txCtx context.Context) error { return s.repo.FinishTurn(txCtx, turnID, status) })
}

func (s *Service) FinalizeTurn(ctx context.Context, sessionID, turnID, status string, raw json.RawMessage) (*domain.Event, error) {
	runtimeEvent, err := domain.ParseRuntimeEvent(raw)
	if err != nil {
		return nil, err
	}
	event := &domain.Event{ID: uuid.NewString(), SessionID: sessionID, TurnID: turnID, Type: runtimeEvent.Type, Payload: raw, CreatedAt: time.Now().UTC()}
	err = s.tx.RunInTx(ctx, func(txCtx context.Context) error {
		sequence, err := s.repo.NextEventSequence(txCtx, sessionID)
		if err != nil {
			return err
		}
		event.Sequence = sequence
		if err := s.repo.AppendEvent(txCtx, event, nil, runtimeEvent.SessionID); err != nil {
			return err
		}
		return s.repo.FinishTurn(txCtx, turnID, status)
	})
	if err != nil {
		return nil, err
	}
	return event, nil
}

func normalizeRole(role, kind any) string {
	value := strings.ToLower(strings.TrimSpace(toString(role)))
	if strings.Contains(value, "assistant") || value == "2" {
		return "assistant"
	}
	if strings.Contains(value, "user") || value == "1" {
		return "user"
	}
	if strings.Contains(strings.ToLower(toString(kind)), "tool") || value == "3" {
		return "tool"
	}
	return "assistant"
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
