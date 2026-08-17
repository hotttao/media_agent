package agent_session

import (
	"context"
	"encoding/json"
	"testing"

	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
)

type fakeRepo struct {
	session  *domain.Session
	messages []domain.Message
	events   []domain.Event
	finished string
}

func (f *fakeRepo) FindLatestActive(context.Context, string) (*domain.Session, error) {
	if f.session == nil {
		return nil, domain.ErrNotFound
	}
	return f.session, nil
}
func (f *fakeRepo) CreateSession(_ context.Context, s *domain.Session) error {
	f.session = s
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
func (f *fakeRepo) ListMessages(context.Context, string) ([]domain.Message, error) {
	return f.messages, nil
}
func (f *fakeRepo) ListEventsAfter(_ context.Context, _ string, sequence int64) ([]domain.Event, error) {
	var result []domain.Event
	for _, event := range f.events {
		if event.Sequence > sequence {
			result = append(result, event)
		}
	}
	return result, nil
}
func (f *fakeRepo) FindCompletedItemEvent(_ context.Context, turnID, itemID, itemKind string) (*domain.Event, error) {
	for i := range f.events {
		event, err := domain.ParseRuntimeEvent(f.events[i].Payload)
		if err == nil && f.events[i].TurnID == turnID && event.ItemID == itemID && event.ItemKind == itemKind {
			return &f.events[i], nil
		}
	}
	return nil, domain.ErrNotFound
}
func (f *fakeRepo) NextMessageSequence(context.Context, string) (int64, error) {
	return int64(len(f.messages) + 1), nil
}
func (f *fakeRepo) NextEventSequence(context.Context, string) (int64, error) {
	return int64(len(f.events) + 1), nil
}
func (f *fakeRepo) CreateTurnAndMessage(_ context.Context, _ *domain.Turn, m *domain.Message) error {
	f.messages = append(f.messages, *m)
	return nil
}
func (f *fakeRepo) AppendEvent(_ context.Context, e *domain.Event, m *domain.Message, runtimeID string) error {
	f.events = append(f.events, *e)
	if m != nil {
		f.messages = append(f.messages, *m)
	}
	if runtimeID != "" {
		f.session.RuntimeSessionID = runtimeID
	}
	return nil
}
func (f *fakeRepo) FinishTurn(_ context.Context, _ string, status string) error {
	f.finished = status
	return nil
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type defaultContext struct{}

func (defaultContext) Default(context.Context, string) (AgentContext, error) {
	return AgentContext{WorkspaceID: "default", AgentID: "codex", AgentType: "codex", ContextVersion: 1}, nil
}
func (defaultContext) Resolve(ctx context.Context, userID, workspaceID, token string) (AgentContext, error) {
	return AgentContext{WorkspaceID: workspaceID, AgentID: "codex", AgentType: "codex", ContextVersion: 1}, nil
}

func TestPersistCompletedItemCreatesFullMessage(t *testing.T) {
	repo := &fakeRepo{session: &domain.Session{ID: "session", UserID: "user"}}
	service := New(repo, directTx{}, defaultContext{})
	raw := json.RawMessage(`{"type":"item.completed","session_id":"runtime-1","item":{"item_id":"i1","kind":"message","role":"assistant","status":"completed","content":[{"type":"text","text":"full answer"}]}}`)

	if _, err := service.PersistEvent(context.Background(), "session", "turn", raw); err != nil {
		t.Fatal(err)
	}
	if len(repo.events) != 1 {
		t.Fatalf("events=%d, want 1", len(repo.events))
	}
	if len(repo.messages) != 1 {
		t.Fatalf("messages=%d, want 1", len(repo.messages))
	}
	if repo.messages[0].Role != "assistant" || string(repo.messages[0].Content) != string(rawItem(raw)) {
		t.Fatalf("unexpected message: %+v", repo.messages[0])
	}
	if repo.session.RuntimeSessionID != "runtime-1" {
		t.Fatalf("runtime session id was not saved")
	}
}

func TestPersistToolEventsDoNotCreateMessagesAndToolCallIsIdempotent(t *testing.T) {
	repo := &fakeRepo{session: &domain.Session{ID: "session", UserID: "user"}}
	service := New(repo, directTx{}, defaultContext{})
	toolCall := json.RawMessage(`{"type":"item.completed","item":{"item_id":"call-item","kind":"tool_call","content":[{"type":"tool_call","call_id":"call"}]}}`)
	toolResult := json.RawMessage(`{"type":"item.completed","item":{"item_id":"result-item","kind":"tool_result","content":[{"type":"tool_result","call_id":"call","output":"ok"}]}}`)

	for _, raw := range []json.RawMessage{toolCall, toolCall, toolResult} {
		if _, err := service.PersistEvent(context.Background(), "session", "turn", raw); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.events) != 2 {
		t.Fatalf("events=%d, want one tool call and one result", len(repo.events))
	}
	if len(repo.messages) != 0 {
		t.Fatalf("tool events created %d messages", len(repo.messages))
	}
}

func TestFinalizeTurnAppendsTerminalEventBeforeFinishing(t *testing.T) {
	repo := &fakeRepo{session: &domain.Session{ID: "session", UserID: "user"}}
	service := New(repo, directTx{}, defaultContext{})
	raw := json.RawMessage(`{"type":"session.ended","reason":"completed"}`)

	event, err := service.FinalizeTurn(context.Background(), "session", "turn", "completed", raw)
	if err != nil {
		t.Fatal(err)
	}
	if event.Sequence != 1 || len(repo.events) != 1 || repo.finished != "completed" {
		t.Fatalf("terminal transaction was incomplete: event=%+v events=%d status=%q", event, len(repo.events), repo.finished)
	}
}

func rawItem(raw json.RawMessage) json.RawMessage {
	var event struct {
		Item json.RawMessage `json:"item"`
	}
	_ = json.Unmarshal(raw, &event)
	return event.Item
}
