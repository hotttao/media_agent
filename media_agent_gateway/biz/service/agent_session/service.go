package agent_session

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
)

type AgentContext struct {
	WorkspaceID, AgentID, AgentType, APIKey, AgentConfigID, MetadataTargetContextVersion string
	ContextVersion                                                                       int64
}

type ContextProvider interface {
	Default(context.Context, string) (AgentContext, error)
	Resolve(context.Context, string, string, string) (AgentContext, error)
}

type Service struct {
	repo        domain.Repository
	contexts    ContextProvider
	credentials domain.CredentialIssuer
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
	if _, err := s.EnsureToken(ctx, created, false); err != nil {
		return nil, err
	}
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
	if _, err := s.EnsureToken(ctx, created, false); err != nil {
		return nil, err
	}
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
	if reader, ok := s.contexts.(interface {
		ResolveVersion(context.Context, string, string, string, int64) (AgentContext, error)
	}); ok {
		return reader.ResolveVersion(ctx, session.UserID, session.WorkspaceID, authToken, session.ContextVersion)
	}
	return s.contexts.Resolve(ctx, session.UserID, session.WorkspaceID, authToken)
}

func (s *Service) Authorize(ctx context.Context, sessionID, userID string) (*domain.Session, error) {
	session, err := s.repo.GetOwned(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	if _, err := s.EnsureToken(ctx, session, true); err != nil {
		return nil, err
	}
	return session, nil
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
		AgentConfigID:  config.AgentConfigID, MetadataTargetContextVersion: config.MetadataTargetContextVersion,
		Status: domain.StatusStarting, BindingPhase: "unbound", ExecutionEpoch: 1, ExpectedExecutionEpoch: 1,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (s *Service) WithCredentials(issuer domain.CredentialIssuer) *Service {
	s.credentials = issuer
	return s
}
func (s *Service) EnsureToken(ctx context.Context, session *domain.Session, persist bool) (string, error) {
	if s.credentials == nil {
		return "", nil
	}
	if session.TokenHash == "" {
		_, hash, key, err := s.credentials.Issue(session)
		if err != nil {
			return "", domain.ErrCredentialUnavailable
		}
		if persist {
			repo, ok := s.repo.(domain.BindingRepository)
			if !ok {
				return "", domain.ErrCredentialUnavailable
			}
			saved, err := repo.EnsureCredential(ctx, session.ID, hash, key)
			if err != nil {
				return "", err
			}
			*session = *saved
		} else {
			session.TokenHash = hash
			session.TokenKeyID = key
		}
	}
	return s.credentials.Reconstruct(session)
}

func (s *Service) ResolveCredential(ctx context.Context, token string) (*domain.Session, error) {
	if token == "" || len(token) > 512 || s.credentials == nil {
		return nil, domain.ErrForbidden
	}
	repo, ok := s.repo.(domain.CredentialRepository)
	if !ok {
		return nil, domain.ErrCredentialUnavailable
	}
	digest := sha256.Sum256([]byte(token))
	v, e := repo.ByTokenHash(ctx, hex.EncodeToString(digest[:]))
	if e != nil {
		return nil, e
	}
	expected, e := s.credentials.Reconstruct(v)
	if e != nil {
		return nil, e
	}
	if subtle.ConstantTimeCompare([]byte(expected), []byte(token)) != 1 || v.Status != domain.StatusActive {
		return nil, domain.ErrForbidden
	}
	return v, nil
}
