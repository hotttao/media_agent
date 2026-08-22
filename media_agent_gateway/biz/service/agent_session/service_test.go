package agent_session

import (
	"context"
	"testing"
	"time"

	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
)

type fakeRepo struct {
	session *domain.Session
}

func (f *fakeRepo) FindLatestUsable(context.Context, string) (*domain.Session, error) {
	if f.session == nil {
		return nil, domain.ErrNotFound
	}
	return f.session, nil
}

func (f *fakeRepo) TransitionStatus(_ context.Context, id, from, to string, updatedAt time.Time) error {
	if f.session == nil || f.session.ID != id || f.session.Status != from {
		return domain.ErrNotFound
	}
	f.session.Status, f.session.UpdatedAt = to, updatedAt
	return nil
}

func (f *fakeRepo) CreateSession(_ context.Context, session *domain.Session) error {
	f.session = session
	return nil
}

func (f *fakeRepo) ListOwned(context.Context, string, string) ([]domain.Session, error) {
	if f.session == nil {
		return nil, nil
	}
	return []domain.Session{*f.session}, nil
}

func (f *fakeRepo) GetOwned(_ context.Context, id, user string) (*domain.Session, error) {
	if f.session == nil {
		return nil, domain.ErrNotFound
	}
	if f.session.ID != id || f.session.UserID != user {
		return nil, domain.ErrForbidden
	}
	return f.session, nil
}

type defaultContext struct{}

func (defaultContext) Default(context.Context, string) (AgentContext, error) {
	return AgentContext{WorkspaceID: "default", AgentID: "codex", AgentType: "codex", ContextVersion: 1}, nil
}

func (defaultContext) Resolve(_ context.Context, _ string, workspaceID, _ string) (AgentContext, error) {
	return AgentContext{WorkspaceID: workspaceID, AgentID: "codex", AgentType: "codex", ContextVersion: 1}, nil
}

func TestDefaultSessionCreatesStableRegistration(t *testing.T) {
	repo := &fakeRepo{}
	service := New(repo, defaultContext{})

	created, err := service.DefaultSession(context.Background(), "user")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := service.DefaultSession(context.Background(), "user")
	if err != nil {
		t.Fatal(err)
	}
	if repeated.ID != created.ID || created.AgentType != "codex" || created.ContextVersion != 1 {
		t.Fatalf("unexpected registration: created=%+v repeated=%+v", created, repeated)
	}
	if created.Status != domain.StatusStarting {
		t.Fatalf("status=%q, want starting", created.Status)
	}
}

func TestAuthorizeRejectsForeignOwner(t *testing.T) {
	repo := &fakeRepo{session: &domain.Session{ID: "session", UserID: "owner"}}
	service := New(repo, defaultContext{})

	if _, err := service.Authorize(context.Background(), "session", "other"); err != domain.ErrForbidden {
		t.Fatalf("error=%v, want forbidden", err)
	}
}
