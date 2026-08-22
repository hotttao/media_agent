package agent_task_dispatch

import (
	"context"
	"errors"
	"time"

	taskdomain "media_agent/media_agent_gateway/biz/domain/agent_task"
)

type Publisher interface {
	Publish(context.Context, string, string) error
}

type Relay struct {
	outbox    taskdomain.OutboxRepository
	publisher Publisher
	now       func() time.Time
}

func NewRelay(outbox taskdomain.OutboxRepository, publisher Publisher) *Relay {
	return &Relay{outbox: outbox, publisher: publisher, now: func() time.Time { return time.Now().UTC() }}
}

func (r *Relay) Flush(ctx context.Context, limit int) error {
	events, err := r.outbox.ListPendingActions(ctx, limit)
	if err != nil {
		return err
	}
	var combined error
	for _, event := range events {
		if err := r.publisher.Publish(ctx, event.EventType, event.AggregateID); err != nil {
			_ = r.outbox.MarkDeliveryFailed(ctx, event.ID, "queue publish failed", r.now())
			combined = errors.Join(combined, err)
			continue
		}
		if err := r.outbox.MarkDelivered(ctx, event.ID, r.now()); err != nil {
			combined = errors.Join(combined, err)
		}
	}
	return combined
}

type Loops struct {
	relay      *Relay
	dispatcher *Dispatcher
	interval   time.Duration
}

func NewLoops(relay *Relay, dispatcher *Dispatcher, interval time.Duration) *Loops {
	if interval <= 0 {
		interval = time.Second
	}
	return &Loops{relay: relay, dispatcher: dispatcher, interval: interval}
}

func (l *Loops) Run(ctx context.Context) {
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	for {
		_ = l.relay.Flush(ctx, 100)
		_ = l.dispatcher.Reconcile(ctx, 100)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
