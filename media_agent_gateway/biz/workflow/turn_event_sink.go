package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
	"media_agent/media_agent_gateway/biz/shared/sessionevents"
)

type eventPersister interface {
	PersistEvent(context.Context, string, string, json.RawMessage) (*domain.Event, error)
}

type eventPublisher interface {
	Publish(string, sessionevents.Event)
}

type turnEventSink struct {
	ctx          context.Context
	sessionID    string
	turnID       string
	persister    eventPersister
	publisher    eventPublisher
	maxToolBytes int
	writeMu      sync.Mutex
	toolResults  *toolResultCoalescer
	completedMu  sync.Mutex
	completed    map[string]struct{}
	terminalMu   sync.Mutex
	terminal     json.RawMessage
}

func newTurnEventSink(ctx context.Context, sessionID, turnID string, persister eventPersister, publisher eventPublisher, maxToolBytes int, window time.Duration) *turnEventSink {
	sink := &turnEventSink{ctx: ctx, sessionID: sessionID, turnID: turnID, persister: persister, publisher: publisher, maxToolBytes: maxToolBytes, completed: make(map[string]struct{})}
	sink.toolResults = newToolResultCoalescer(window, sink.persistAndPublish)
	return sink
}

func (s *turnEventSink) Accept(raw json.RawMessage) error {
	event, err := domain.ParseRuntimeEvent(raw)
	if err != nil {
		return err
	}
	if event.IsTransient() {
		s.publisher.Publish(s.sessionID, sessionevents.Event{Payload: append([]byte(nil), raw...)})
		return nil
	}
	if event.IsToolResult() {
		normalized, err := domain.NormalizeToolResult(raw, s.maxToolBytes)
		if err != nil {
			return err
		}
		key := event.CallID
		if key == "" {
			key = event.ItemID
		}
		if key == "" {
			return fmt.Errorf("tool result has no call_id or item_id")
		}
		return s.toolResults.Accept(key, normalized)
	}
	if event.IsTerminal() {
		if err := s.toolResults.Flush(); err != nil {
			return err
		}
		s.terminalMu.Lock()
		s.terminal = append(json.RawMessage(nil), raw...)
		s.terminalMu.Unlock()
		return nil
	}
	if event.IsToolCall() && event.ItemID != "" {
		s.completedMu.Lock()
		defer s.completedMu.Unlock()
		_, seen := s.completed[event.ItemID]
		if seen {
			return nil
		}
		if err := s.persistAndPublish(raw); err != nil {
			return err
		}
		s.completed[event.ItemID] = struct{}{}
		return nil
	}
	return s.persistAndPublish(raw)
}

func (s *turnEventSink) Flush() error { return s.toolResults.Flush() }

func (s *turnEventSink) TerminalReason() string {
	s.terminalMu.Lock()
	defer s.terminalMu.Unlock()
	var envelope struct {
		Reason string `json:"reason"`
	}
	if len(s.terminal) > 0 && json.Unmarshal(s.terminal, &envelope) == nil {
		return envelope.Reason
	}
	return ""
}

func (s *turnEventSink) Terminal(reason string) json.RawMessage {
	s.terminalMu.Lock()
	defer s.terminalMu.Unlock()
	if len(s.terminal) > 0 {
		var envelope struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(s.terminal, &envelope)
		if reason == "completed" || envelope.Reason == reason {
			return append(json.RawMessage(nil), s.terminal...)
		}
	}
	raw, _ := json.Marshal(map[string]any{"type": "session.ended", "reason": reason})
	return raw
}

func (s *turnEventSink) persistAndPublish(raw json.RawMessage) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	event, err := s.persister.PersistEvent(context.WithoutCancel(s.ctx), s.sessionID, s.turnID, raw)
	if err != nil {
		return err
	}
	s.publisher.Publish(s.sessionID, sessionevents.Event{Sequence: event.Sequence, Payload: append([]byte(nil), raw...)})
	return nil
}
