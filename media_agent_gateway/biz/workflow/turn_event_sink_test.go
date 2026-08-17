package workflow

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
	"media_agent/media_agent_gateway/biz/shared/sessionevents"
)

type recordingEventStore struct {
	mu     sync.Mutex
	events []json.RawMessage
}

func (s *recordingEventStore) PersistEvent(ctx context.Context, sessionID, turnID string, raw json.RawMessage) (*domain.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, append(json.RawMessage(nil), raw...))
	return &domain.Event{SessionID: sessionID, TurnID: turnID, Sequence: int64(len(s.events))}, nil
}

func TestTurnEventSinkFinishesPersistenceAfterRuntimeContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &recordingEventStore{}
	sink := newTurnEventSink(ctx, "session", "turn", store, &recordingPublisher{}, 96, time.Hour)
	toolCall := json.RawMessage(`{"type":"item.completed","item":{"item_id":"tool","kind":"tool_call","content":[]}}`)

	if err := sink.Accept(toolCall); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 1 {
		t.Fatalf("event was lost after runtime cancellation: %d", len(store.events))
	}
}

type recordingPublisher struct {
	mu     sync.Mutex
	events []sessionevents.Event
}

func (p *recordingPublisher) Publish(_ string, event sessionevents.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
}

func TestTurnEventSinkSeparatesTransientAndDurableEvents(t *testing.T) {
	store := &recordingEventStore{}
	publisher := &recordingPublisher{}
	sink := newTurnEventSink(context.Background(), "session", "turn", store, publisher, 96, time.Hour)

	delta := json.RawMessage(`{"type":"item.delta","item_id":"message","delta":{"text":"hi"}}`)
	firstResult := json.RawMessage(`{"type":"item.completed","item":{"item_id":"result","kind":"tool_result","content":[{"type":"tool_result","call_id":"call","output":"first"}]}}`)
	latestResult := json.RawMessage(`{"type":"item.completed","item":{"item_id":"result","kind":"tool_result","content":[{"type":"tool_result","call_id":"call","output":"latest"}]}}`)
	toolCall := json.RawMessage(`{"type":"item.completed","item":{"item_id":"tool","kind":"tool_call","content":[]}}`)
	terminal := json.RawMessage(`{"type":"session.ended","reason":"completed"}`)

	for _, raw := range []json.RawMessage{delta, firstResult, latestResult, toolCall, toolCall, terminal} {
		if err := sink.Accept(raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := sink.Flush(); err != nil {
		t.Fatal(err)
	}

	if len(store.events) != 3 {
		t.Fatalf("expected first/latest tool result and one tool call, got %d events", len(store.events))
	}
	if len(publisher.events) != 4 {
		t.Fatalf("expected one transient and three durable broadcasts, got %d", len(publisher.events))
	}
	if publisher.events[0].Sequence != 0 {
		t.Fatalf("transient frame advanced durable cursor: %d", publisher.events[0].Sequence)
	}
	if got := string(sink.Terminal("completed")); got != string(terminal) {
		t.Fatalf("terminal event was not retained for atomic finalization: %s", got)
	}
	var failedEnd struct {
		Reason string `json:"reason"`
	}
	failedRaw := sink.Terminal("error")
	if err := json.Unmarshal(failedRaw, &failedEnd); err != nil || failedEnd.Reason != "error" {
		t.Fatalf("terminal reason was not replaced after failure: %s", failedRaw)
	}
	for _, raw := range store.events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Type == "item.delta" || envelope.Type == "session.ended" {
			t.Fatalf("unexpected event persisted by sink: %s", envelope.Type)
		}
	}
}
