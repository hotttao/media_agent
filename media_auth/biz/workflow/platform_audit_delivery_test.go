package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "media_agent/media_auth/biz/domain/platform_token"
)

type auditTxMarker struct{}
type auditTestTx struct {
	calls, failAt int
	active        bool
}

func (tx *auditTestTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	tx.calls++
	if tx.calls == tx.failAt {
		return errors.New("private transaction diagnostic")
	}
	tx.active = true
	defer func() { tx.active = false }()
	return fn(context.WithValue(ctx, auditTxMarker{}, true))
}

type auditTestEntry struct {
	event       domain.AuditEvent
	lease       string
	until, next time.Time
	attempts    int
	delivered   bool
}
type auditTestOutbox struct {
	entries            []*auditTestEntry
	confirmed, retried int
}

func (o *auditTestOutbox) ClaimAudit(ctx context.Context, lease string, now, until time.Time, limit int) ([]domain.AuditDelivery, error) {
	if ctx.Value(auditTxMarker{}) != true {
		return nil, errors.New("claim outside tx")
	}
	for _, e := range o.entries {
		if !e.delivered && !e.next.After(now) && (e.lease == "" || !e.until.After(now)) {
			e.lease = lease
			e.until = until
			e.attempts++
			return []domain.AuditDelivery{{Event: e.event, LeaseID: lease, LeaseUntil: until, Attempts: e.attempts}}, nil
		}
	}
	return nil, nil
}
func (o *auditTestOutbox) ConfirmAudit(ctx context.Context, id, lease string, now time.Time) (bool, error) {
	if ctx.Value(auditTxMarker{}) != true {
		return false, errors.New("confirm outside tx")
	}
	for _, e := range o.entries {
		if e.event.ID == id && e.lease == lease && !e.delivered && e.until.After(now) {
			e.delivered = true
			o.confirmed++
			return true, nil
		}
	}
	return false, nil
}
func (o *auditTestOutbox) RetryAudit(ctx context.Context, id, lease string, now, next time.Time) (bool, error) {
	if ctx.Value(auditTxMarker{}) != true {
		return false, errors.New("retry outside tx")
	}
	for _, e := range o.entries {
		if e.event.ID == id && e.lease == lease && !e.delivered && e.until.After(now) {
			e.lease = ""
			e.next = next
			o.retried++
			return true, nil
		}
	}
	return false, nil
}

type auditTestSink struct {
	tx      *auditTestTx
	publish func(context.Context, domain.AuditEvent) error
	ids     []string
}

func (s *auditTestSink) Publish(ctx context.Context, e domain.AuditEvent) error {
	if s.tx.active || ctx.Value(auditTxMarker{}) != nil {
		panic("external audit publication held transaction")
	}
	s.ids = append(s.ids, e.ID)
	if s.publish != nil {
		return s.publish(ctx, e)
	}
	return nil
}
func auditEntry(id string, at time.Time) *auditTestEntry {
	return &auditTestEntry{event: domain.AuditEvent{ID: id, UserID: "owner", CredentialID: "credential", Operation: "issued", CallerService: domain.CallerAuth, Purpose: domain.PurposeLogin, OccurredAt: at}}
}
func auditWorker(t *testing.T, tx *auditTestTx, o *auditTestOutbox, s *auditTestSink, now *time.Time) *PlatformAuditDelivery {
	t.Helper()
	w, err := NewPlatformAuditDelivery(tx, o, s, PlatformAuditOptions{Now: func() time.Time { return *now }})
	if err != nil {
		t.Fatal(err)
	}
	return w
}
func TestPlatformAuditFailureContinuesAndRetriesSameEventAfterRestart(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	o := &auditTestOutbox{entries: []*auditTestEntry{auditEntry("a", now), auditEntry("b", now)}}
	tx := &auditTestTx{}
	s := &auditTestSink{tx: tx, publish: func(_ context.Context, e domain.AuditEvent) error {
		if e.ID == "a" {
			return errors.New("private sink diagnostic")
		}
		return nil
	}}
	w := auditWorker(t, tx, o, s, &now)
	if err := w.ProcessPending(t.Context()); err != ErrPlatformAuditDelivery || len(s.ids) != 2 || !o.entries[1].delivered || o.entries[0].delivered || o.retried != 1 {
		t.Fatal("one failure dropped facts or blocked another event")
	}
	now = now.Add(2 * time.Second)
	s.publish = nil
	w = auditWorker(t, tx, o, s, &now)
	if w.ProcessPending(t.Context()) != nil || !o.entries[0].delivered || len(s.ids) != 3 || s.ids[2] != "a" || o.entries[0].attempts != 2 {
		t.Fatal("restart did not preserve event identity")
	}
}
func TestPlatformAuditUnknownConfirmAndCancellationRetainLease(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		o := &auditTestOutbox{entries: []*auditTestEntry{auditEntry("a", now)}}
		tx := &auditTestTx{failAt: 2}
		s := &auditTestSink{tx: tx}
		ctx, cancel := context.WithCancel(t.Context())
		if cancelled {
			tx.failAt = 0
			s.publish = func(context.Context, domain.AuditEvent) error { cancel(); return nil }
		}
		w := auditWorker(t, tx, o, s, &now)
		if w.ProcessPending(ctx) != ErrPlatformAuditDelivery || o.entries[0].delivered || o.entries[0].lease == "" || o.confirmed != 0 {
			t.Fatal("unknown/cancelled publication confirmed")
		}
		cancel()
		now = now.Add(31 * time.Second)
		s.publish = nil
		tx.failAt = 0
		w = auditWorker(t, tx, o, s, &now)
		if w.ProcessPending(t.Context()) != nil || len(s.ids) != 2 || s.ids[0] != s.ids[1] || !o.entries[0].delivered {
			t.Fatal("expired lease did not replay stable event")
		}
	}
}
func TestPlatformAuditClaimFailureDoesNotPublishAndSlowFailureDoesNotLoopSameEvent(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	o := &auditTestOutbox{entries: []*auditTestEntry{auditEntry("a", now), auditEntry("b", now)}}
	tx := &auditTestTx{failAt: 1}
	s := &auditTestSink{tx: tx}
	w := auditWorker(t, tx, o, s, &now)
	if w.ProcessPending(t.Context()) != ErrPlatformAuditDelivery || len(s.ids) != 0 {
		t.Fatal("unknown claim published")
	}
	tx.failAt = 0
	s.publish = func(_ context.Context, e domain.AuditEvent) error {
		if e.ID == "a" {
			now = now.Add(31 * time.Second)
			return errors.New("private slow failure")
		}
		return nil
	}
	if w.ProcessPending(t.Context()) != ErrPlatformAuditDelivery || len(s.ids) != 2 || !o.entries[1].delivered || o.entries[0].delivered {
		t.Fatal("expired publication looped or blocked a good event")
	}
}
func TestPlatformAuditOptionsRejectUnsafeLeaseAndBounds(t *testing.T) {
	tx, o := &auditTestTx{}, &auditTestOutbox{}
	s := &auditTestSink{tx: tx}
	for _, options := range []PlatformAuditOptions{{LeaseTTL: time.Second, PublishTimeout: time.Second}, {LeaseTTL: 6 * time.Minute}, {BatchSize: 101}, {BatchSize: -1}, {Interval: -time.Second}} {
		if _, err := NewPlatformAuditDelivery(tx, o, s, options); err != ErrPlatformAuditDelivery {
			t.Fatal("unsafe delivery configuration accepted")
		}
	}
}
