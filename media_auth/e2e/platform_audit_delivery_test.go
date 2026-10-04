//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"media_agent/media_auth/biz/dal"
	repository "media_agent/media_auth/biz/dal/repo/platform_token"
	domain "media_agent/media_auth/biz/domain/platform_token"
	"media_agent/media_auth/biz/workflow"
)

func TestPlatformAuditPostgresSkipLockedLeaseCASAndTerminalHistory(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	seedUser(t, db, "other", "other")
	core := platformCore(t, db, nil)
	if core.EnsurePlatformToken(t.Context(), "owner") != nil || core.EnsurePlatformToken(t.Context(), "other") != nil {
		t.Fatal("audit fixture creation failed")
	}
	r := repository.NewRepo(db.Gorm)
	tx := dal.NewTxRunner(db.Gorm)
	now := time.Now().UTC().Add(time.Second).Truncate(time.Microsecond)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	claimed := make(chan domain.AuditDelivery, 1)
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- tx.RunInTx(ctx, func(ctx context.Context) error {
			claims, err := r.ClaimAudit(ctx, "lease-first", now, now.Add(30*time.Second), 1)
			if err != nil {
				return err
			}
			if len(claims) != 1 {
				return errors.New("first claim missing")
			}
			claimed <- claims[0]
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	var first domain.AuditDelivery
	select {
	case first = <-claimed:
	case <-ctx.Done():
		t.Fatal("first transaction did not claim")
	}
	var second []domain.AuditDelivery
	skipCtx, stop := context.WithTimeout(ctx, 3*time.Second)
	err := tx.RunInTx(skipCtx, func(ctx context.Context) error {
		var err error
		second, err = r.ClaimAudit(ctx, "lease-second", now, now.Add(30*time.Second), 1)
		return err
	})
	stop()
	close(release)
	if err != nil || len(second) != 1 || first.Event.ID == second[0].Event.ID {
		t.Fatal("claim did not skip held row")
	}
	if err := <-firstDone; err != nil {
		t.Fatal("first claim did not commit")
	}
	confirm := func(id, lease string, at time.Time) bool {
		t.Helper()
		var ok bool
		if tx.RunInTx(t.Context(), func(ctx context.Context) error {
			var err error
			ok, err = r.ConfirmAudit(ctx, id, lease, at)
			return err
		}) != nil {
			t.Fatal("confirmation query failed")
		}
		return ok
	}
	if confirm(first.Event.ID, "wrong-lease", now) || confirm(first.Event.ID, first.LeaseID, now.Add(30*time.Second)) {
		t.Fatal("wrong or expired lease confirmed")
	}
	later := now.Add(31 * time.Second)
	var reclaimed []domain.AuditDelivery
	if tx.RunInTx(t.Context(), func(ctx context.Context) error {
		var err error
		reclaimed, err = r.ClaimAudit(ctx, "lease-new", later, later.Add(30*time.Second), 2)
		return err
	}) != nil || len(reclaimed) != 2 {
		t.Fatal("expired leases not recovered")
	}
	for _, c := range reclaimed {
		if c.Attempts != 2 {
			t.Fatal("claim attempts not preserved")
		}
	}
	if confirm(first.Event.ID, first.LeaseID, now) {
		t.Fatal("old confirmation overwrote new lease")
	}
	var retried bool
	if tx.RunInTx(t.Context(), func(ctx context.Context) error {
		var err error
		retried, err = r.RetryAudit(ctx, first.Event.ID, "lease-new", later, later.Add(5*time.Second))
		return err
	}) != nil || !retried {
		t.Fatal("retry could not be scheduled")
	}
	var due []domain.AuditDelivery
	if tx.RunInTx(t.Context(), func(ctx context.Context) error {
		var err error
		due, err = r.ClaimAudit(ctx, "lease-early", later, later.Add(30*time.Second), 2)
		return err
	}) != nil || len(due) != 0 {
		t.Fatal("retry ignored delay or active lease")
	}
	ready := later.Add(6 * time.Second)
	if tx.RunInTx(t.Context(), func(ctx context.Context) error {
		var err error
		due, err = r.ClaimAudit(ctx, "lease-final", ready, ready.Add(30*time.Second), 1)
		return err
	}) != nil || len(due) != 1 || due[0].Event.ID != first.Event.ID {
		t.Fatal("due retry changed event identity")
	}
	if !confirm(first.Event.ID, "lease-final", ready) || confirm(first.Event.ID, "lease-final", ready) {
		t.Fatal("delivered terminal state was overwritten")
	}
	if _, err := r.ClaimAudit(t.Context(), "no-tx", now, now.Add(time.Second), 1); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("claim outside transaction accepted")
	}
	if _, err := r.ConfirmAudit(t.Context(), first.Event.ID, "no-tx", now); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("confirm outside transaction accepted")
	}
	if _, err := r.RetryAudit(t.Context(), first.Event.ID, "no-tx", now, now.Add(time.Second)); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("retry outside transaction accepted")
	}
	if db.Gorm.Exec("UPDATE media_auth.platform_token_audit_outbox SET attempts=2147483647 WHERE delivered_at IS NULL").Error != nil {
		t.Fatal("saturation fixture failed")
	}
	far := ready.Add(time.Hour)
	if tx.RunInTx(t.Context(), func(ctx context.Context) error {
		var err error
		due, err = r.ClaimAudit(ctx, "lease-saturated", far, far.Add(30*time.Second), 10)
		return err
	}) != nil || len(due) != 1 || due[0].Event.ID == first.Event.ID || due[0].Attempts != 2147483647 {
		t.Fatal("terminal claim or attempt overflow")
	}
	if db.Gorm.Exec("UPDATE media_auth.platform_token_audit_outbox SET lease_until=clock_timestamp()-interval '1 minute' WHERE id=?", due[0].Event.ID).Error != nil {
		t.Fatal("expired DB clock fixture failed")
	}
	if confirm(due[0].Event.ID, "lease-saturated", time.Now().Add(-2*time.Minute)) {
		t.Fatal("stale process timestamp revived expired lease")
	}
}

type auditConfirmUnavailableTx struct {
	delegate dal.TxRunner
	calls    int
}

func (tx *auditConfirmUnavailableTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	tx.calls++
	if tx.calls == 2 {
		return errors.New("private confirmation fault")
	}
	return tx.delegate.RunInTx(ctx, fn)
}

type realPlatformAuditSink struct {
	delegate  domain.AuditSink
	failOwner bool
}

func (s *realPlatformAuditSink) Publish(ctx context.Context, e domain.AuditEvent) error {
	if dal.InTransaction(ctx) {
		return errors.New("publication held transaction")
	}
	if s.failOwner && e.UserID == "owner" {
		return errors.New("private sink fault")
	}
	return s.delegate.Publish(ctx, e)
}
func readAuditFileEvents(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal("durable audit file unavailable")
	}
	defer f.Close()
	var result []map[string]any
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		if len(scan.Bytes()) == 0 {
			continue
		}
		var record map[string]any
		if json.Unmarshal(scan.Bytes(), &record) != nil {
			t.Fatal("invalid completed audit line")
		}
		if record["event"] != "platform_token_audit" || len(record) != 8 {
			t.Fatal("noncanonical or payload-bearing audit record")
		}
		result = append(result, record)
	}
	if scan.Err() != nil {
		t.Fatal("audit read failed")
	}
	return result
}
func TestPlatformAuditPostgresUnknownConfirmFileReopenAndStableEventReplay(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	core := platformCore(t, db, nil)
	if core.EnsurePlatformToken(t.Context(), "owner") != nil {
		t.Fatal("fixture failed")
	}
	r := repository.NewRepo(db.Gorm)
	current, err := r.GetCurrent(t.Context(), "owner")
	if err != nil {
		t.Fatal("fixture credential missing")
	}
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	file, err := workflow.OpenPlatformAuditFileSink(path)
	if err != nil {
		t.Fatal("file sink failed")
	}
	now := time.Now().UTC().Add(time.Second)
	options := workflow.PlatformAuditOptions{Now: func() time.Time { return now }}
	w, err := workflow.NewPlatformAuditDelivery(&auditConfirmUnavailableTx{delegate: dal.NewTxRunner(db.Gorm)}, r, &realPlatformAuditSink{delegate: file}, options)
	if err != nil {
		t.Fatal(err)
	}
	if w.ProcessPending(t.Context()) != workflow.ErrPlatformAuditDelivery {
		t.Fatal("unknown confirmation reported complete")
	}
	if file.Close() != nil {
		t.Fatal("sink close failed")
	}
	first := readAuditFileEvents(t, path)
	if len(first) != 1 {
		t.Fatal("publication not actually persisted")
	}
	now = now.Add(31 * time.Second)
	file, err = workflow.OpenPlatformAuditFileSink(path)
	if err != nil {
		t.Fatal("sink reopen failed")
	}
	defer file.Close()
	w, err = workflow.NewPlatformAuditDelivery(dal.NewTxRunner(db.Gorm), r, &realPlatformAuditSink{delegate: file}, options)
	if err != nil {
		t.Fatal(err)
	}
	if w.ProcessPending(t.Context()) != nil || w.ProcessPending(t.Context()) != nil {
		t.Fatal("restart recovery failed")
	}
	replayed := readAuditFileEvents(t, path)
	if len(replayed) != 2 || replayed[0]["event_id"] != replayed[1]["event_id"] || replayed[0]["credential_id"] != current.ID || replayed[1]["user_id"] != "owner" {
		t.Fatal("recovery changed audit identity or attribution")
	}
	var delivered int64
	if db.Gorm.Table("media_auth.platform_token_audit_outbox").Where("delivered_at IS NOT NULL AND lease_id IS NULL AND attempts=2").Count(&delivered).Error != nil || delivered != 1 {
		t.Fatal("delivery fact not recovered")
	}
	if after, err := r.GetCurrent(t.Context(), "owner"); err != nil || after.ID != current.ID {
		t.Fatal("audit delivery mutated credentials")
	}
}
func TestPlatformAuditPostgresOneFailureContinuesOtherFactsThenRetry(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	seedUser(t, db, "other", "other")
	core := platformCore(t, db, nil)
	if core.EnsurePlatformToken(t.Context(), "owner") != nil || core.EnsurePlatformToken(t.Context(), "other") != nil {
		t.Fatal("fixture failed")
	}
	r := repository.NewRepo(db.Gorm)
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	file, err := workflow.OpenPlatformAuditFileSink(path)
	if err != nil {
		t.Fatal("file sink failed")
	}
	defer file.Close()
	now := time.Now().UTC().Add(time.Second)
	sink := &realPlatformAuditSink{delegate: file, failOwner: true}
	options := workflow.PlatformAuditOptions{Now: func() time.Time { return now }}
	w, err := workflow.NewPlatformAuditDelivery(dal.NewTxRunner(db.Gorm), r, sink, options)
	if err != nil {
		t.Fatal(err)
	}
	if w.ProcessPending(t.Context()) != workflow.ErrPlatformAuditDelivery {
		t.Fatal("failed event incorrectly completed")
	}
	first := readAuditFileEvents(t, path)
	if len(first) != 1 || first[0]["user_id"] != "other" {
		t.Fatal("one failure blocked another owner")
	}
	var pending int64
	if db.Gorm.Table("media_auth.platform_token_audit_outbox").Where("user_id='owner' AND delivered_at IS NULL AND lease_id IS NULL AND next_attempt_at > ?", now).Count(&pending).Error != nil || pending != 1 {
		t.Fatal("failed fact lost or retry not deferred")
	}
	now = now.Add(2 * time.Second)
	sink.failOwner = false
	w, err = workflow.NewPlatformAuditDelivery(dal.NewTxRunner(db.Gorm), r, sink, options)
	if err != nil || w.ProcessPending(t.Context()) != nil {
		t.Fatal("retry did not converge")
	}
	if got := readAuditFileEvents(t, path); len(got) != 2 || got[1]["user_id"] != "owner" {
		t.Fatal("deferred owner not durably published")
	}
}
