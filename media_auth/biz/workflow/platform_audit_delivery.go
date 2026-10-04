package workflow

import (
	"context"
	"errors"
	"time"

	domain "media_agent/media_auth/biz/domain/platform_token"
	"media_agent/media_auth/biz/shared/ids"
)

var ErrPlatformAuditDelivery = errors.New("platform token audit delivery incomplete")

type PlatformAuditOptions struct {
	Now                                func() time.Time
	NewLeaseID                         func() (string, error)
	LeaseTTL, PublishTimeout, Interval time.Duration
	BatchSize                          int
}
type PlatformAuditDelivery struct {
	tx      TransactionRunner
	outbox  domain.AuditOutbox
	sink    domain.AuditSink
	options PlatformAuditOptions
}

func NewPlatformAuditDelivery(tx TransactionRunner, outbox domain.AuditOutbox, sink domain.AuditSink, o PlatformAuditOptions) (*PlatformAuditDelivery, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.NewLeaseID == nil {
		o.NewLeaseID = ids.NewChecked
	}
	if o.LeaseTTL == 0 {
		o.LeaseTTL = 30 * time.Second
	}
	if o.PublishTimeout == 0 {
		o.PublishTimeout = 5 * time.Second
	}
	if o.Interval == 0 {
		o.Interval = time.Second
	}
	if o.BatchSize == 0 {
		o.BatchSize = 20
	}
	if tx == nil || outbox == nil || sink == nil || o.LeaseTTL <= 0 || o.LeaseTTL > 5*time.Minute || o.PublishTimeout <= 0 || o.PublishTimeout >= o.LeaseTTL || o.Interval <= 0 || o.Interval > time.Minute || o.BatchSize < 1 || o.BatchSize > 100 {
		return nil, ErrPlatformAuditDelivery
	}
	return &PlatformAuditDelivery{tx: tx, outbox: outbox, sink: sink, options: o}, nil
}
func (w *PlatformAuditDelivery) now() time.Time {
	return w.options.Now().UTC().Truncate(time.Microsecond)
}

func (w *PlatformAuditDelivery) ProcessPending(ctx context.Context) error {
	if w == nil {
		return ErrPlatformAuditDelivery
	}
	failed := false
	seen := make(map[string]bool)
	for i := 0; i < w.options.BatchSize; i++ {
		if ctx.Err() != nil {
			return ErrPlatformAuditDelivery
		}
		lease, err := w.options.NewLeaseID()
		if err != nil || !domain.ValidIdentity(lease) {
			return ErrPlatformAuditDelivery
		}
		now := w.now()
		var claims []domain.AuditDelivery
		err = w.tx.RunInTx(ctx, func(txCtx context.Context) error {
			var err error
			claims, err = w.outbox.ClaimAudit(txCtx, lease, now, now.Add(w.options.LeaseTTL), 1)
			return err
		})
		if err != nil {
			return ErrPlatformAuditDelivery
		}
		if len(claims) == 0 {
			break
		}
		if len(claims) != 1 {
			return ErrPlatformAuditDelivery
		}
		c := claims[0]
		if !c.Event.Valid() || c.LeaseID != lease || !c.LeaseUntil.After(w.now()) || c.Attempts < 1 {
			return ErrPlatformAuditDelivery
		}
		if seen[c.Event.ID] {
			// A slow failed/unknown publication may have outlived its lease.
			// Do not publish the same event repeatedly within this pass.
			at := w.now()
			err = w.tx.RunInTx(ctx, func(txCtx context.Context) error {
				_, err := w.outbox.RetryAudit(txCtx, c.Event.ID, lease, at, at.Add(time.Second*time.Duration(1<<min(c.Attempts-1, 8))))
				return err
			})
			if err != nil {
				return ErrPlatformAuditDelivery
			}
			failed = true
			continue
		}
		seen[c.Event.ID] = true
		publishCtx, cancel := context.WithTimeout(ctx, w.options.PublishTimeout)
		err = w.sink.Publish(publishCtx, c.Event)
		if publishCtx.Err() != nil {
			err = ErrPlatformAuditDelivery
		}
		cancel()
		if ctx.Err() != nil {
			return ErrPlatformAuditDelivery
		}
		at := w.now()
		ok := false
		if err != nil {
			failed = true
			delay := time.Second * time.Duration(1<<min(c.Attempts-1, 8))
			err = w.tx.RunInTx(ctx, func(txCtx context.Context) error {
				var err error
				ok, err = w.outbox.RetryAudit(txCtx, c.Event.ID, lease, at, at.Add(delay))
				return err
			})
		} else {
			err = w.tx.RunInTx(ctx, func(txCtx context.Context) error {
				var err error
				ok, err = w.outbox.ConfirmAudit(txCtx, c.Event.ID, lease, at)
				return err
			})
		}
		if err != nil || !ok {
			failed = true
		}
	}
	if failed {
		return ErrPlatformAuditDelivery
	}
	return nil
}
func (w *PlatformAuditDelivery) Run(ctx context.Context) {
	if w == nil {
		return
	}
	ticker := time.NewTicker(w.options.Interval)
	defer ticker.Stop()
	for {
		_ = w.ProcessPending(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
