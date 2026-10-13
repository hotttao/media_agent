package metadata_update

import (
	"context"
	"log"
	d "media_agent/media_control/biz/domain/agent_configuration"
	"time"
)

type Publisher interface {
	Publish(context.Context, string, string, []byte) error
}
type Relay struct {
	repo      d.Repository
	publisher Publisher
	tx        interface {
		RunInTx(context.Context, func(context.Context) error) error
	}
}

func NewRelay(repo d.Repository, publisher Publisher, tx interface {
	RunInTx(context.Context, func(context.Context) error) error
}) *Relay {
	return &Relay{repo, publisher, tx}
}
func (r *Relay) Once(ctx context.Context) error {
	var notices []d.Notice
	err := r.tx.RunInTx(ctx, func(txCtx context.Context) error {
		var err error
		notices, err = r.repo.ClaimNotices(txCtx, 32)
		return err
	})
	if err != nil {
		return err
	}
	for _, notice := range notices {
		attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := r.publisher.Publish(attempt, notice.RoutingKey, notice.EventID, notice.Payload)
		cancel()
		if err := r.repo.FinishNotice(ctx, notice, err == nil); err != nil {
			return err
		}
	}
	return nil
}
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := r.Once(ctx); err != nil {
			log.Print("metadata outbox reconciliation deferred")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
