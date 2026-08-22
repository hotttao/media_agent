package agent_session

import (
	"context"
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

type Service struct {
	repo     domain.Repository
	contexts ContextProvider
}

func New(repo domain.Repository, contexts ContextProvider) *Service {
	return &Service{repo: repo, contexts: contexts}
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
	created := newSession(userID, title, config)
	if err := s.repo.CreateSession(ctx, created); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Service) DefaultSession(ctx context.Context, userID string) (*domain.Session, error) {
	found, err := s.repo.FindLatestUsable(ctx, userID)
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
	created := newSession(userID, "New session", config)
	if err := s.repo.CreateSession(ctx, created); err != nil {
		if existing, findErr := s.repo.FindLatestUsable(ctx, userID); findErr == nil {
			return existing, nil
		}
		return nil, err
	}
	return created, nil
}

func (s *Service) TransitionStatus(ctx context.Context, session *domain.Session, to string) error {
	if session.Status == to {
		return nil
	}
	now := time.Now().UTC()
	if err := s.repo.TransitionStatus(ctx, session.ID, session.Status, to, now); err != nil {
		return err
	}
	session.Status, session.UpdatedAt = to, now
	return nil
}

func (s *Service) List(ctx context.Context, userID, workspaceID string) ([]domain.Session, error) {
	return s.repo.ListOwned(ctx, userID, workspaceID)
}

func (s *Service) RuntimeContext(ctx context.Context, session *domain.Session, authToken string) (AgentContext, error) {
	return s.contexts.Resolve(ctx, session.UserID, session.WorkspaceID, authToken)
}

func (s *Service) Authorize(ctx context.Context, sessionID, userID string) (*domain.Session, error) {
	return s.repo.GetOwned(ctx, sessionID, userID)
}

func newSession(userID, title string, config AgentContext) *domain.Session {
	now := time.Now().UTC()
	return &domain.Session{
		ID:             uuid.NewString(),
		UserID:         userID,
		WorkspaceID:    config.WorkspaceID,
		AgentID:        config.AgentID,
		AgentType:      config.AgentType,
		Title:          title,
		ContextVersion: config.ContextVersion,
		Status:         domain.StatusStarting,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}
