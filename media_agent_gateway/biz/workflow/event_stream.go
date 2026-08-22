package workflow

import (
	"context"
	"encoding/json"
	"time"

	"media_agent/media_agent_gateway/biz/shared/sessionevents"
)

type StreamEvent struct {
	Sequence  int64
	Payload   []byte
	Heartbeat bool
}

type EventStream struct {
	turns          *RunTurn
	source         sessionevents.Source
	reconcileEvery time.Duration
	heartbeatEvery time.Duration
}

func NewEventStream(turns *RunTurn, source sessionevents.Source) *EventStream {
	return &EventStream{turns: turns, source: source, reconcileEvery: time.Second, heartbeatEvery: 15 * time.Second}
}

// Stream opens the live subscription before replay. Durable events advance only
// contiguously; a high live sequence is withheld until Agent replay fills its gap.
func (s *EventStream) Stream(ctx context.Context, sessionID, userID string, after int64, emit func(StreamEvent) error) error {
	var subscription sessionevents.Subscription
	if s.source != nil {
		opened, err := s.source.Open(ctx, sessionID)
		if err == nil {
			subscription = opened
		}
	}
	defer subscription.Close()
	cursor := after
	replay := func() error {
		events, err := s.turns.EventsAfter(ctx, sessionID, userID, cursor)
		if err != nil {
			return err
		}
		for _, event := range events {
			if event.Sequence <= cursor {
				continue
			}
			if event.Sequence != cursor+1 {
				break
			}
			if err := emit(StreamEvent{Sequence: event.Sequence, Payload: event.Payload}); err != nil {
				return err
			}
			cursor = event.Sequence
		}
		return nil
	}
	if err := replay(); err != nil {
		return err
	}
	reconcile := time.NewTicker(s.reconcileEvery)
	defer reconcile.Stop()
	heartbeat := time.NewTicker(s.heartbeatEvery)
	defer heartbeat.Stop()
	events := subscription.Events
	for {
		select {
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if event.Sequence == 0 {
				if err := emit(StreamEvent{Payload: event.Payload}); err != nil {
					return err
				}
				continue
			}
			if event.Sequence <= cursor {
				continue
			}
			if event.Sequence > cursor+1 {
				if err := replay(); err != nil {
					continue
				}
				if event.Sequence != cursor+1 {
					continue
				}
			}
			if err := emit(StreamEvent{Sequence: event.Sequence, Payload: event.Payload}); err != nil {
				return err
			}
			cursor = event.Sequence
			if terminalEvent(event.Payload) {
				_ = replay()
			}
		case <-reconcile.C:
			_ = replay()
		case <-heartbeat.C:
			if err := emit(StreamEvent{Heartbeat: true}); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func terminalEvent(payload []byte) bool {
	var envelope struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(payload, &envelope) != nil {
		return false
	}
	switch envelope.Type {
	case "session.ended", "turn.completed", "turn.failed", "turn.interrupted":
		return true
	default:
		return false
	}
}
