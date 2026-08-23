package teamwork_change

import (
	"context"
	"errors"
	"time"

	domain "media_agent/media_control/biz/domain/teamwork"
)

type Publisher interface {
	Publish(context.Context, string, string, string, int64, string) error
}

type Relay struct {
	outbox    domain.OutboxRepository
	publisher Publisher
	now       func() time.Time
}

func NewRelay(outbox domain.OutboxRepository, publisher Publisher) *Relay {
	return &Relay{outbox: outbox, publisher: publisher, now: func() time.Time { return time.Now().UTC() }}
}

func (r *Relay) Flush(ctx context.Context, limit int) error {
	events, err := r.outbox.ListPending(ctx, limit)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err = r.publisher.Publish(ctx, event.ID, event.WorkspaceID, event.TopologyID, event.TopologyVersion, event.Status); err != nil {
			markErr := r.outbox.MarkDeliveryFailed(ctx, event.ID, err.Error(), r.now())
			return errors.Join(err, markErr)
		}
		if err = r.outbox.MarkDelivered(ctx, event.ID, r.now()); err != nil {
			return err
		}
	}
	return nil
}

func (r *Relay) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_ = r.Flush(ctx, 100)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
