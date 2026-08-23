package agent_task

import (
	"context"
	"errors"
	"media_agent/media_agent_gateway/biz/dal"
	session "media_agent/media_agent_gateway/biz/domain/agent_session"
	domain "media_agent/media_agent_gateway/biz/domain/agent_task"
	"testing"
	"time"
)

type fakeRepo struct{ value *domain.Task }

func (f *fakeRepo) Create(_ context.Context, v *domain.Task) error { x := *v; f.value = &x; return nil }
func (f *fakeRepo) FindByKey(_ context.Context, o, s, r, k string) (*domain.Task, error) {
	if f.value == nil || f.value.RequestedBy != o || f.value.Source != s || f.value.SourceResourceID != r || f.value.IdempotencyKey != k {
		return nil, domain.ErrNotFound
	}
	x := *f.value
	return &x, nil
}
func (f *fakeRepo) ListOwned(context.Context, string, string, domain.Status) ([]domain.Task, error) {
	if f.value == nil {
		return nil, nil
	}
	return []domain.Task{*f.value}, nil
}
func (f *fakeRepo) GetOwned(_ context.Context, id, o string) (*domain.Task, error) {
	if f.value == nil || f.value.ID != id {
		return nil, domain.ErrNotFound
	}
	if f.value.RequestedBy != o {
		return nil, domain.ErrForbidden
	}
	x := *f.value
	return &x, nil
}
func (f *fakeRepo) Cancel(_ context.Context, v *domain.Task, _ []domain.Status) error {
	x := *v
	f.value = &x
	return nil
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

var _ dal.TxRunner = directTx{}

type fakeSessions struct{ value *session.Session }

func (f *fakeSessions) FindLatestUsable(context.Context, string) (*session.Session, error) {
	return nil, session.ErrNotFound
}
func (f *fakeSessions) ListOwned(context.Context, string, string) ([]session.Session, error) {
	return nil, nil
}
func (f *fakeSessions) CreateSession(context.Context, *session.Session) error { return nil }
func (f *fakeSessions) GetOwned(_ context.Context, id, o string) (*session.Session, error) {
	if f.value == nil || f.value.ID != id {
		return nil, session.ErrNotFound
	}
	if f.value.UserID != o {
		return nil, session.ErrForbidden
	}
	return f.value, nil
}
func (f *fakeSessions) TransitionStatus(context.Context, string, string, string, time.Time) error {
	return nil
}

type allowWorkspace struct{ err error }

func (a allowWorkspace) AuthorizeWorkspace(context.Context, string, string, string) error {
	return a.err
}

type allowTopology struct{ err error }

func (a allowTopology) AuthorizeTopology(context.Context, string, string, int64) error { return a.err }

type serviceOutbox struct{ events []domain.OutboxEvent }

func (f *serviceOutbox) Enqueue(_ context.Context, event *domain.OutboxEvent) error {
	f.events = append(f.events, *event)
	return nil
}
func (f *serviceOutbox) ListPendingActions(context.Context, int) ([]domain.OutboxEvent, error) {
	return nil, nil
}
func (f *serviceOutbox) MarkDelivered(context.Context, string, time.Time) error { return nil }
func (f *serviceOutbox) MarkDeliveryFailed(context.Context, string, string, time.Time) error {
	return nil
}

func TestCreateIsIdempotentAndCancel(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, nil, directTx{}, &fakeSessions{}, allowWorkspace{}, nil)
	cmd := CreateCommand{Owner: "user-1", WorkspaceID: "ws-1", Source: "manual", SourceResourceID: "request-1", IdempotencyKey: "key-1", InputJSON: `{"message":"hi"}`}
	first, err := svc.Create(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Create(context.Background(), cmd)
	if err != nil || second.ID != first.ID {
		t.Fatalf("idempotent create failed: %+v %v", second, err)
	}
	cancelled, err := svc.Cancel(context.Background(), first.ID, "user-1")
	if err != nil || cancelled.Status != domain.StatusCancelled || cancelled.FinishedAt == nil {
		t.Fatalf("cancel failed: %+v %v", cancelled, err)
	}
}
func TestCreateRejectsSessionWorkspaceMismatch(t *testing.T) {
	svc := New(&fakeRepo{}, nil, directTx{}, &fakeSessions{value: &session.Session{ID: "s-1", UserID: "user-1", WorkspaceID: "ws-2"}}, allowWorkspace{}, nil)
	_, err := svc.Create(context.Background(), CreateCommand{Owner: "user-1", WorkspaceID: "ws-1", SessionID: "s-1", Source: "manual", SourceResourceID: "r", IdempotencyKey: "k", InputJSON: "{}"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestCreateAndRunningCancelWriteOutboxActions(t *testing.T) {
	repo, outbox := &fakeRepo{}, &serviceOutbox{}
	svc := New(repo, outbox, directTx{}, &fakeSessions{}, allowWorkspace{}, nil)
	created, err := svc.Create(context.Background(), CreateCommand{Owner: "user-1", WorkspaceID: "ws-1", Source: "manual", SourceResourceID: "r", IdempotencyKey: "k", InputJSON: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	if len(outbox.events) != 1 || outbox.events[0].EventType != "agent_task.dispatch" {
		t.Fatalf("events=%+v", outbox.events)
	}
	repo.value.Status = domain.StatusRunning
	if _, err := svc.Cancel(context.Background(), created.ID, "user-1"); err != nil {
		t.Fatal(err)
	}
	if len(outbox.events) != 3 || outbox.events[1].EventType != "agent_task.cancel" || outbox.events[2].EventType != "agent_task.cancelled" {
		t.Fatalf("events=%+v", outbox.events)
	}
}

func TestCreateRequiresAllowedTopologyVersion(t *testing.T) {
	command := CreateCommand{Owner: "user-1", WorkspaceID: "ws-1", Source: "manual", SourceResourceID: "r", IdempotencyKey: "k", InputJSON: "{}", TopologyID: "topology-1", TopologyVersion: 2}
	svc := New(&fakeRepo{}, nil, directTx{}, &fakeSessions{}, allowWorkspace{}, allowTopology{})
	created, err := svc.Create(context.Background(), command)
	if err != nil || created.TopologyID != "topology-1" || created.TopologyVersion != 2 {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	svc = New(&fakeRepo{}, nil, directTx{}, &fakeSessions{}, allowWorkspace{}, allowTopology{err: errors.New("stale")})
	if _, err = svc.Create(context.Background(), command); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale topology error=%v", err)
	}
	command.TopologyVersion = 0
	if _, err = svc.Create(context.Background(), command); err == nil {
		t.Fatal("partial topology reference was accepted")
	}
}
