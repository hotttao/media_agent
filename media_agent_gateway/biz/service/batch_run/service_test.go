package batch_run

import (
	"context"
	"errors"
	"testing"
	"time"

	"media_agent/media_agent_gateway/biz/dal"
	sessiondomain "media_agent/media_agent_gateway/biz/domain/agent_session"
	domain "media_agent/media_agent_gateway/biz/domain/batch_run"
)

type fakeRepo struct{ value *domain.BatchRun }

func (f *fakeRepo) Create(_ context.Context, value *domain.BatchRun) error {
	copy := *value
	copy.Items = append([]domain.Item(nil), value.Items...)
	f.value = &copy
	return nil
}
func (f *fakeRepo) FindByKey(_ context.Context, owner, workspace, key string) (*domain.BatchRun, error) {
	if f.value == nil || f.value.RequestedBy != owner || f.value.WorkspaceID != workspace || f.value.IdempotencyKey != key {
		return nil, domain.ErrNotFound
	}
	return f.value, nil
}
func (f *fakeRepo) ListOwned(context.Context, string, string, domain.Status) ([]domain.BatchRun, error) {
	if f.value == nil {
		return nil, nil
	}
	return []domain.BatchRun{*f.value}, nil
}
func (f *fakeRepo) GetOwned(_ context.Context, id, owner string) (*domain.BatchRun, error) {
	if f.value == nil || f.value.ID != id {
		return nil, domain.ErrNotFound
	}
	if f.value.RequestedBy != owner {
		return nil, domain.ErrForbidden
	}
	return f.value, nil
}
func (f *fakeRepo) Cancel(_ context.Context, value *domain.BatchRun) error {
	for i := range value.Items {
		if value.Items[i].AgentTaskID == "" {
			value.Items[i].Status = domain.ItemCancelled
			value.CancelledCount++
		}
	}
	f.value = value
	return nil
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

var _ dal.TxRunner = directTx{}

type fakeSessions struct{ session *sessiondomain.Session }

func (f *fakeSessions) FindLatestUsable(context.Context, string) (*sessiondomain.Session, error) {
	return nil, sessiondomain.ErrNotFound
}
func (f *fakeSessions) ListOwned(context.Context, string, string) ([]sessiondomain.Session, error) {
	return nil, nil
}
func (f *fakeSessions) CreateSession(context.Context, *sessiondomain.Session) error { return nil }
func (f *fakeSessions) GetOwned(_ context.Context, id, owner string) (*sessiondomain.Session, error) {
	if f.session == nil || f.session.ID != id {
		return nil, sessiondomain.ErrNotFound
	}
	if f.session.UserID != owner {
		return nil, sessiondomain.ErrForbidden
	}
	return f.session, nil
}
func (f *fakeSessions) TransitionStatus(context.Context, string, string, string, time.Time) error {
	return nil
}

func TestCreateIsIdempotentAndCancelFreezesItems(t *testing.T) {
	repo := &fakeRepo{}
	service := New(repo, nil, directTx{}, &fakeSessions{session: &sessiondomain.Session{ID: "session", UserID: "user", WorkspaceID: "workspace"}})
	command := CreateCommand{Owner: "user", WorkspaceID: "workspace", IdempotencyKey: "key", Items: []domain.Item{{ItemKey: "one", SessionID: "session", InputJSON: `{"message":"hi"}`}}}
	first, err := service.Create(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(context.Background(), command)
	if err != nil || second.ID != first.ID {
		t.Fatalf("second=%+v error=%v", second, err)
	}
	cancelled, err := service.Cancel(context.Background(), first.ID, "user")
	if err != nil || cancelled.Status != domain.StatusCancelled || cancelled.Items[0].Status != domain.ItemCancelled {
		t.Fatalf("cancelled=%+v error=%v", cancelled, err)
	}
}

func TestCreateRejectsSessionWorkspaceMismatch(t *testing.T) {
	service := New(&fakeRepo{}, nil, directTx{}, &fakeSessions{session: &sessiondomain.Session{ID: "session", UserID: "user", WorkspaceID: "other"}})
	_, err := service.Create(context.Background(), CreateCommand{Owner: "user", WorkspaceID: "workspace", IdempotencyKey: "key", Items: []domain.Item{{ItemKey: "one", SessionID: "session", InputJSON: "{}"}}})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error=%v", err)
	}
}
