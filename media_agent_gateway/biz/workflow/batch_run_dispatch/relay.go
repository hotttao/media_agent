package batch_run_dispatch

import (
	"context"
	"errors"
	"time"

	"media_agent/media_agent_gateway/biz/dal"
	domain "media_agent/media_agent_gateway/biz/domain/batch_run"
)

type Publisher interface {
	PublishBatch(context.Context, string, string) error
}

type Relay struct {
	outbox domain.OutboxRepository
	queue  Publisher
	now    func() time.Time
}

func NewRelay(outbox domain.OutboxRepository, queue Publisher) *Relay {
	return &Relay{outbox: outbox, queue: queue, now: func() time.Time { return time.Now().UTC() }}
}
func (r *Relay) Flush(ctx context.Context, limit int) error {
	events, err := r.outbox.ListPendingActions(ctx, limit)
	if err != nil {
		return err
	}
	var combined error
	for _, event := range events {
		if err := r.queue.PublishBatch(ctx, event.Action, event.BatchRunID); err != nil {
			_ = r.outbox.MarkDeliveryFailed(ctx, event.ID, "queue publish failed", r.now())
			combined = errors.Join(combined, err)
			continue
		}
		combined = errors.Join(combined, r.outbox.MarkDelivered(ctx, event.ID, r.now()))
	}
	return combined
}

type Projector struct {
	source  domain.TaskTerminalSource
	batches domain.DispatchRepository
	tx      dal.TxRunner
	now     func() time.Time
}

func NewProjector(source domain.TaskTerminalSource, batches domain.DispatchRepository, tx dal.TxRunner) *Projector {
	return &Projector{source: source, batches: batches, tx: tx, now: func() time.Time { return time.Now().UTC() }}
}
func (p *Projector) Flush(ctx context.Context, limit int) error {
	events, err := p.source.ListPendingBatchTerminal(ctx, limit)
	if err != nil {
		return err
	}
	var combined error
	for _, event := range events {
		err := p.tx.RunInTx(ctx, func(tx context.Context) error { return p.batches.ApplyTaskTerminal(tx, event, p.now()) })
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			combined = errors.Join(combined, err)
			continue
		}
		combined = errors.Join(combined, p.source.MarkDelivered(ctx, event.EventID, p.now()))
	}
	return combined
}

type Loops struct {
	relay     *Relay
	projector *Projector
	interval  time.Duration
}

func NewLoops(relay *Relay, projector *Projector, interval time.Duration) *Loops {
	if interval <= 0 {
		interval = time.Second
	}
	return &Loops{relay: relay, projector: projector, interval: interval}
}
func (l *Loops) Run(ctx context.Context) {
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	for {
		_ = l.relay.Flush(ctx, 100)
		_ = l.projector.Flush(ctx, 100)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
