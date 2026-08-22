package workflow

import (
	"context"
	"reflect"
	"testing"
	"time"

	"media_agent/media_agent_gateway/biz/shared/client"
	"media_agent/media_agent_gateway/biz/shared/sessionevents"
)

type orderedSource struct {
	opened bool
	events chan sessionevents.Event
}

func (s *orderedSource) Open(context.Context, string) (sessionevents.Subscription, error) {
	s.opened = true
	return sessionevents.Subscription{Events: s.events}, nil
}
func (*orderedSource) Close() error { return nil }

func TestEventStreamSubscribesBeforeReplayAndMergesWithoutDuplicates(t *testing.T) {
	flow, _, runtime := newWorkflowFixture("active")
	source := &orderedSource{events: make(chan sessionevents.Event, 4)}
	source.events <- sessionevents.Event{Sequence: 2, Payload: []byte(`{"type":"two-live"}`)}
	runtime.replay = func(after int64) []client.UniversalEvent {
		if !source.opened {
			t.Fatal("replay happened before subscription")
		}
		switch after {
		case 0:
			return []client.UniversalEvent{{Sequence: 1, Payload: []byte(`{"type":"one"}`)}, {Sequence: 2, Payload: []byte(`{"type":"two"}`)}}
		case 2:
			return []client.UniversalEvent{{Sequence: 3, Payload: []byte(`{"type":"three"}`)}}
		default:
			return nil
		}
	}
	stream := NewEventStream(flow, source)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make([]int64, 0, 3)
	source.events <- sessionevents.Event{Sequence: 3, Payload: []byte(`{"type":"turn.completed"}`)}
	err := stream.Stream(ctx, "session", "user", 0, func(event StreamEvent) error {
		if event.Sequence > 0 {
			got = append(got, event.Sequence)
		}
		if len(got) == 3 {
			cancel()
		}
		return nil
	})
	if err != context.Canceled {
		t.Fatalf("error=%v", err)
	}
	if !reflect.DeepEqual(got, []int64{1, 2, 3}) {
		t.Fatalf("sequences=%v", got)
	}
}

func TestEventStreamDoesNotEmitAcrossSequenceGap(t *testing.T) {
	flow, _, runtime := newWorkflowFixture("active")
	source := &orderedSource{events: make(chan sessionevents.Event, 1)}
	source.events <- sessionevents.Event{Sequence: 3, Payload: []byte(`{"type":"three"}`)}
	runtime.replay = func(after int64) []client.UniversalEvent {
		if after == 0 {
			return []client.UniversalEvent{{Sequence: 1, Payload: []byte(`{"type":"one"}`)}}
		}
		return nil
	}
	stream := NewEventStream(flow, source)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	got := make([]int64, 0, 2)
	err := stream.Stream(ctx, "session", "user", 0, func(event StreamEvent) error {
		if event.Sequence > 0 {
			got = append(got, event.Sequence)
		}
		return nil
	})
	if err != context.DeadlineExceeded {
		t.Fatalf("error=%v", err)
	}
	if !reflect.DeepEqual(got, []int64{1}) {
		t.Fatalf("gap was crossed: %v", got)
	}
}
