package teamwork_change

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "media_agent/media_control/biz/domain/teamwork"
)

type relayOutbox struct {
	events              []domain.ChangeEvent
	delivered, failedID string
}

func (f *relayOutbox) Enqueue(context.Context, *domain.ChangeEvent) error { return nil }
func (f *relayOutbox) ListPending(context.Context, int) ([]domain.ChangeEvent, error) {
	return f.events, nil
}
func (f *relayOutbox) MarkDelivered(_ context.Context, id string, _ time.Time) error {
	f.delivered = id
	return nil
}
func (f *relayOutbox) MarkDeliveryFailed(_ context.Context, id, _ string, _ time.Time) error {
	f.failedID = id
	return nil
}

type relayPublisher struct {
	err   error
	calls []string
}

func (f *relayPublisher) Publish(_ context.Context, id, _, _ string, _ int64, _ string) error {
	f.calls = append(f.calls, id)
	return f.err
}

func TestRelayDeliversAndConfirms(t *testing.T) {
	outbox := &relayOutbox{events: []domain.ChangeEvent{{ID: "event-1", TopologyID: "topology-1", TopologyVersion: 2, Status: "active"}}}
	publisher := &relayPublisher{}
	if err := NewRelay(outbox, publisher).Flush(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if outbox.delivered != "event-1" || len(publisher.calls) != 1 {
		t.Fatalf("outbox=%+v calls=%v", outbox, publisher.calls)
	}
}

func TestRelayStopsAtFailureToPreserveOrder(t *testing.T) {
	outbox := &relayOutbox{events: []domain.ChangeEvent{{ID: "event-1"}, {ID: "event-2"}}}
	publisher := &relayPublisher{err: errors.New("gateway unavailable")}
	if err := NewRelay(outbox, publisher).Flush(context.Background(), 10); err == nil {
		t.Fatal("publish failure was ignored")
	}
	if outbox.failedID != "event-1" || len(publisher.calls) != 1 {
		t.Fatalf("outbox=%+v calls=%v", outbox, publisher.calls)
	}
}
