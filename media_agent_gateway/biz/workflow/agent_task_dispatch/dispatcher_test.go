package agent_task_dispatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"media_agent/media_agent_gateway/biz/dal"
	sessiondomain "media_agent/media_agent_gateway/biz/domain/agent_session"
	taskdomain "media_agent/media_agent_gateway/biz/domain/agent_task"
	sessionservice "media_agent/media_agent_gateway/biz/service/agent_session"
	"media_agent/media_agent_gateway/biz/shared/client"
)

type fakeTasks struct {
	task            *taskdomain.Task
	cancelOnWaiting bool
}

func (f *fakeTasks) Get(_ context.Context, id string) (*taskdomain.Task, error) {
	if f.task == nil || f.task.ID != id {
		return nil, taskdomain.ErrNotFound
	}
	copy := *f.task
	return &copy, nil
}
func (f *fakeTasks) Claim(_ context.Context, id string, now time.Time) (*taskdomain.Task, error) {
	if f.task == nil || f.task.ID != id {
		return nil, taskdomain.ErrNotFound
	}
	if f.task.Status == taskdomain.StatusRunning && f.task.TurnID == "" {
		copy := *f.task
		return &copy, nil
	}
	if f.task.Status != taskdomain.StatusQueued && f.task.Status != taskdomain.StatusRetryWait {
		return nil, taskdomain.ErrConflict
	}
	f.task.Status, f.task.UpdatedAt = taskdomain.StatusRunning, now
	f.task.Attempt++
	copy := *f.task
	return &copy, nil
}
func (f *fakeTasks) MarkWaitingTurn(_ context.Context, id, turnID string, now time.Time) error {
	if f.task.ID != id || f.task.Status != taskdomain.StatusRunning {
		return taskdomain.ErrConflict
	}
	if f.cancelOnWaiting {
		f.task.Status, f.task.TurnID, f.task.UpdatedAt = taskdomain.StatusCancelled, turnID, now
		return taskdomain.ErrCancelled
	}
	f.task.Status, f.task.TurnID, f.task.UpdatedAt = taskdomain.StatusWaitingTurn, turnID, now
	return nil
}
func (f *fakeTasks) MarkRetryWait(_ context.Context, id, code, message string, now time.Time) error {
	if f.task.ID != id || f.task.Status != taskdomain.StatusRunning {
		return taskdomain.ErrConflict
	}
	f.task.Status, f.task.ErrorCode, f.task.ErrorMessage, f.task.UpdatedAt = taskdomain.StatusRetryWait, code, message, now
	return nil
}
func (f *fakeTasks) MarkTerminal(_ context.Context, id string, status taskdomain.Status, result, code, message string, now time.Time) error {
	if f.task.ID != id || (f.task.Status != taskdomain.StatusRunning && f.task.Status != taskdomain.StatusWaitingTurn) {
		return taskdomain.ErrConflict
	}
	f.task.Status, f.task.ResultRefJSON, f.task.ErrorCode, f.task.ErrorMessage, f.task.UpdatedAt = status, result, code, message, now
	f.task.FinishedAt = &now
	return nil
}
func (f *fakeTasks) ListWaitingTurn(context.Context, int) ([]taskdomain.Task, error) {
	if f.task != nil && f.task.Status == taskdomain.StatusWaitingTurn {
		return []taskdomain.Task{*f.task}, nil
	}
	return nil, nil
}

type fakeOutbox struct {
	events    []taskdomain.OutboxEvent
	delivered map[string]bool
	attempts  map[string]int
}

func (f *fakeOutbox) Enqueue(_ context.Context, event *taskdomain.OutboxEvent) error {
	copy := *event
	if copy.ID == "" {
		copy.ID = copy.EventType
	}
	f.events = append(f.events, copy)
	return nil
}
func (f *fakeOutbox) ListPendingActions(context.Context, int) ([]taskdomain.OutboxEvent, error) {
	var result []taskdomain.OutboxEvent
	for _, event := range f.events {
		if !f.delivered[event.ID] && (event.EventType == "agent_task.dispatch" || event.EventType == "agent_task.cancel") {
			result = append(result, event)
		}
	}
	return result, nil
}
func (f *fakeOutbox) MarkDelivered(_ context.Context, id string, _ time.Time) error {
	if f.delivered == nil {
		f.delivered = map[string]bool{}
	}
	f.delivered[id] = true
	return nil
}
func (f *fakeOutbox) MarkDeliveryFailed(_ context.Context, id, _ string, _ time.Time) error {
	if f.attempts == nil {
		f.attempts = map[string]int{}
	}
	f.attempts[id]++
	return nil
}

type fakeTx struct{}

func (fakeTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

var _ dal.TxRunner = fakeTx{}

type fakeSessionService struct{ session *sessiondomain.Session }

func (f *fakeSessionService) Authorize(_ context.Context, id, owner string) (*sessiondomain.Session, error) {
	if f.session == nil || f.session.ID != id {
		return nil, sessiondomain.ErrNotFound
	}
	if f.session.UserID != owner {
		return nil, sessiondomain.ErrForbidden
	}
	return f.session, nil
}
func (f *fakeSessionService) RuntimeContext(context.Context, *sessiondomain.Session, string) (sessionservice.AgentContext, error) {
	return sessionservice.AgentContext{WorkspaceID: "workspace", AgentID: "agent", AgentType: "codex", APIKey: "provider-secret", ContextVersion: 3}, nil
}
func (f *fakeSessionService) TransitionStatus(_ context.Context, value *sessiondomain.Session, to string) error {
	value.Status = to
	return nil
}

type fakeRuntime struct {
	submitCount int
	submit      client.SubmitTurnInput
	submitErr   error
	turnStatus  string
	interrupted bool
}

func (f *fakeRuntime) RegisterRuntimeSession(_ context.Context, in client.RegisterRuntimeSessionInput) (*client.RuntimeSession, error) {
	return &client.RuntimeSession{SessionID: in.SessionID, ContextVersion: in.ContextVersion, Status: "active"}, nil
}
func (f *fakeRuntime) GetRuntimeSession(context.Context, string) (*client.RuntimeSession, error) {
	return &client.RuntimeSession{}, nil
}
func (f *fakeRuntime) SubmitTurn(_ context.Context, in client.SubmitTurnInput) (*client.Turn, error) {
	f.submitCount++
	f.submit = in
	if f.submitErr != nil {
		return nil, f.submitErr
	}
	status := f.turnStatus
	if status == "" {
		status = "queued"
	}
	return &client.Turn{TurnID: "turn-1", SessionID: in.SessionID, IdempotencyKey: in.IdempotencyKey, Status: status}, nil
}
func (f *fakeRuntime) GetTurn(_ context.Context, sessionID, turnID string) (*client.Turn, error) {
	status := f.turnStatus
	if status == "" {
		status = "queued"
	}
	return &client.Turn{TurnID: turnID, SessionID: sessionID, Status: status}, nil
}
func (f *fakeRuntime) InterruptTurn(context.Context, string, string) (bool, error) {
	f.interrupted = true
	return true, nil
}
func (f *fakeRuntime) ListMessages(context.Context, string, int) ([]client.Message, error) {
	return nil, nil
}
func (f *fakeRuntime) ReplayEvents(context.Context, string, int64, int) ([]client.UniversalEvent, error) {
	return nil, nil
}
func (f *fakeRuntime) ListAgentRequests(context.Context, string, string) ([]client.AgentRequest, error) {
	return nil, nil
}
func (f *fakeRuntime) ResolveAgentRequest(context.Context, string, string, string, map[string]any) (*client.AgentRequest, error) {
	return nil, nil
}

func fixture(status taskdomain.Status) (*Dispatcher, *fakeTasks, *fakeOutbox, *fakeRuntime) {
	tasks := &fakeTasks{task: &taskdomain.Task{ID: "task-1", WorkspaceID: "workspace", RequestedBy: "user", SessionID: "session", InputJSON: `{"message":"hello"}`, Status: status}}
	outbox := &fakeOutbox{}
	runtime := &fakeRuntime{}
	sessions := &fakeSessionService{session: &sessiondomain.Session{ID: "session", UserID: "user", WorkspaceID: "workspace", AgentID: "agent", AgentType: "codex", ContextVersion: 3, Status: sessiondomain.StatusActive}}
	return NewDispatcher(tasks, outbox, fakeTx{}, sessions, client.Registry{"codex": runtime}), tasks, outbox, runtime
}

func TestDispatchIsIdempotentAcrossDuplicateDelivery(t *testing.T) {
	dispatcher, tasks, _, runtime := fixture(taskdomain.StatusQueued)
	if err := dispatcher.Process(context.Background(), "agent_task.dispatch", "task-1"); err != nil {
		t.Fatal(err)
	}
	if tasks.task.Status != taskdomain.StatusWaitingTurn || tasks.task.TurnID != "turn-1" || runtime.submit.IdempotencyKey != "task-1" || runtime.submit.SessionToken != "" {
		t.Fatalf("task=%+v submit=%+v", tasks.task, runtime.submit)
	}
	if err := dispatcher.Process(context.Background(), "agent_task.dispatch", "task-1"); err != nil {
		t.Fatal(err)
	}
	if runtime.submitCount != 1 {
		t.Fatalf("duplicate delivery submitted %d Turns", runtime.submitCount)
	}
}

func TestDispatchRetriesTransientSubmissionAndConvergesTerminalTurn(t *testing.T) {
	dispatcher, tasks, outbox, runtime := fixture(taskdomain.StatusQueued)
	runtime.submitErr = errors.New("connection reset")
	if err := dispatcher.Process(context.Background(), "agent_task.dispatch", "task-1"); err == nil {
		t.Fatal("expected transient failure")
	}
	if tasks.task.Status != taskdomain.StatusRetryWait {
		t.Fatalf("status=%s", tasks.task.Status)
	}
	runtime.submitErr, runtime.turnStatus = nil, "completed"
	if err := dispatcher.Process(context.Background(), "agent_task.dispatch", "task-1"); err != nil {
		t.Fatal(err)
	}
	if tasks.task.Status != taskdomain.StatusCompleted || len(outbox.events) != 1 || outbox.events[0].EventType != "agent_task.completed" {
		t.Fatalf("task=%+v events=%+v", tasks.task, outbox.events)
	}
}

func TestCancelJobInterruptsAssociatedTurn(t *testing.T) {
	dispatcher, tasks, _, runtime := fixture(taskdomain.StatusCancelled)
	tasks.task.TurnID = "turn-1"
	runtime.turnStatus = "running"
	if err := dispatcher.Process(context.Background(), "agent_task.cancel", "task-1"); err != nil {
		t.Fatal(err)
	}
	if !runtime.interrupted {
		t.Fatal("associated Turn was not interrupted")
	}
}

func TestCancelRacingWithSubmitPersistsAndInterruptsAcceptedTurn(t *testing.T) {
	dispatcher, tasks, _, runtime := fixture(taskdomain.StatusQueued)
	tasks.cancelOnWaiting = true
	runtime.turnStatus = "running"
	if err := dispatcher.Process(context.Background(), "agent_task.dispatch", "task-1"); err != nil {
		t.Fatal(err)
	}
	if tasks.task.Status != taskdomain.StatusCancelled || tasks.task.TurnID != "turn-1" || !runtime.interrupted {
		t.Fatalf("task=%+v interrupted=%v", tasks.task, runtime.interrupted)
	}
}

type fakePublisher struct {
	err   error
	calls int
}

func (f *fakePublisher) Publish(context.Context, string, string) error { f.calls++; return f.err }

func TestRelayRecordsFailureAndMarksSuccessfulDelivery(t *testing.T) {
	outbox := &fakeOutbox{events: []taskdomain.OutboxEvent{{ID: "event", AggregateID: "task", EventType: "agent_task.dispatch"}}}
	publisher := &fakePublisher{err: errors.New("queue unavailable")}
	relay := NewRelay(outbox, publisher)
	if err := relay.Flush(context.Background(), 10); err == nil || outbox.attempts["event"] != 1 {
		t.Fatalf("error=%v attempts=%v", err, outbox.attempts)
	}
	publisher.err = nil
	if err := relay.Flush(context.Background(), 10); err != nil || !outbox.delivered["event"] {
		t.Fatalf("error=%v delivered=%v", err, outbox.delivered)
	}
}
