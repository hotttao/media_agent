//go:build e2e

package e2e

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"media_agent/media_auth/biz/dal"
	authrepo "media_agent/media_auth/biz/dal/repo/auth"
	repo "media_agent/media_auth/biz/dal/repo/platform_token"
	domain "media_agent/media_auth/biz/domain/platform_token"
	service "media_agent/media_auth/biz/service/platform_token"
	"media_agent/media_auth/e2e/fixtures"
)

// This test verifier is not a transport authentication implementation.
type platformCaller struct{}

func (platformCaller) VerifyPlatformCaller(context.Context) (string, error) {
	return domain.CallerGateway, nil
}

func platformCore(t *testing.T, db *fixtures.Database, audit domain.AuditWriter) *service.Service {
	t.Helper()
	r := repo.NewRepo(db.Gorm)
	if audit == nil {
		audit = r
	}
	return service.NewService(r, audit, authrepo.NewPlatformUserReader(db.Gorm), dal.NewTxRunner(db.Gorm), platformTestRing(t), platformCaller{}, service.Options{})
}

func platformAuditCount(t *testing.T, db *fixtures.Database, expected int64) {
	t.Helper()
	var n int64
	if db.Gorm.Table("media_auth.platform_token_audit_outbox").Count(&n).Error != nil || n != expected {
		t.Fatal("unexpected audit history or partial commit")
	}
}

func TestPlatformTokenCoreLifecycleCurrentUserAndAuditAtomicity(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	seedUser(t, db, "other", "other")
	r := repo.NewRepo(db.Gorm)
	s := platformCore(t, db, nil)
	for i := 0; i < 2; i++ {
		if err := s.EnsurePlatformToken(t.Context(), "owner"); err != nil {
			t.Fatal("bootstrap failed")
		}
	}
	migrationCounts(t, db, 0, 1)
	platformAuditCount(t, db, 1)
	first, err := s.AcquirePlatformToken(t.Context(), "owner", domain.PurposeOffline)
	if err != nil || first == "" {
		t.Fatal("authorized acquire failed")
	}
	principal, err := s.IntrospectPlatformToken(t.Context(), first, domain.PurposeOffline)
	if err != nil || !principal.Active || principal.UserID != "owner" || principal.CredentialID == "" {
		t.Fatal("valid custody was not introspected")
	}
	oldID := principal.CredentialID
	platformAuditCount(t, db, 3)
	if secret, err := s.AcquirePlatformToken(t.Context(), "owner", domain.PurposeLogin); secret != "" || !errors.Is(err, service.ErrDenied) {
		t.Fatal("external caller obtained login purpose")
	}
	platformAuditCount(t, db, 3)
	if err := s.EnsurePlatformToken(t.Context(), "other"); err != nil {
		t.Fatal("other owner bootstrap failed")
	}
	other, err := r.GetCurrent(t.Context(), "other")
	if err != nil {
		t.Fatal("other custody missing")
	}
	second, err := s.RotatePlatformToken(t.Context(), "owner", domain.PurposeOffline, nil)
	if err != nil || second == "" || second == first {
		t.Fatal("rotation failed")
	}
	if p, err := s.IntrospectPlatformToken(t.Context(), first, domain.PurposeOffline); err != nil || p.Active {
		t.Fatal("rotated secret still active")
	}
	if p, err := s.IntrospectPlatformToken(t.Context(), second, domain.PurposeOffline); err != nil || !p.Active || p.CredentialID == oldID {
		t.Fatal("new secret not active")
	}
	old, err := r.GetIncludingRevoked(t.Context(), oldID)
	if err != nil || old.RevokedAt == nil {
		t.Fatal("rotation discarded revoked history")
	}
	if err := db.Gorm.Exec("UPDATE media_auth.users SET status='disabled' WHERE id='owner'").Error; err != nil {
		t.Fatal("fixture disable failed")
	}
	if p, err := s.IntrospectPlatformToken(t.Context(), second, domain.PurposeOffline); err != nil || p.Active {
		t.Fatal("disabled owner credential remained usable")
	}
	if secret, err := s.AcquirePlatformToken(t.Context(), "owner", domain.PurposeOffline); secret != "" || !errors.Is(err, service.ErrDenied) {
		t.Fatal("disabled owner acquired Secret")
	}
	if err := s.EnsurePlatformToken(t.Context(), "owner"); !errors.Is(err, service.ErrDenied) {
		t.Fatal("disabled owner bootstrapped")
	}
	if secret, err := s.RotatePlatformToken(t.Context(), "owner", domain.PurposeOffline, nil); secret != "" || !errors.Is(err, service.ErrDenied) {
		t.Fatal("disabled owner rotated")
	}
	if err := s.RevokeAllForUser(t.Context(), "owner"); !errors.Is(err, service.ErrUnavailable) {
		t.Fatal("revocation outside caller transaction accepted")
	}
	if err := s.RevokePlatformToken(t.Context(), "owner"); err != nil {
		t.Fatal("disabled owner cleanup failed")
	}
	platformAuditCount(t, db, 7)
	if err := s.RevokePlatformToken(t.Context(), "owner"); err != nil {
		t.Fatal("repeat revocation failed")
	}
	platformAuditCount(t, db, 7)
	if _, err := r.GetCurrent(t.Context(), "owner"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("revoked custody remained current")
	}
	if current, err := r.GetCurrent(t.Context(), "other"); err != nil || current.ID != other.ID {
		t.Fatal("one owner lifecycle modified another")
	}
	for _, owner := range []string{"unknown", "other"} {
		if owner == "other" {
			if err := db.Gorm.Exec("UPDATE media_auth.users SET status='deleted' WHERE id='other'").Error; err != nil {
				t.Fatal("fixture deletion failed")
			}
		}
		if err := s.EnsurePlatformToken(t.Context(), owner); !errors.Is(err, service.ErrDenied) {
			t.Fatal("missing or deleted owner issued credential")
		}
	}
	var secretColumns int
	if db.Gorm.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema='media_auth' AND table_name='platform_token_audit_outbox' AND column_name IN ('token','secret','plaintext','ciphertext','token_hash','payload')`).Scan(&secretColumns).Error != nil || secretColumns != 0 {
		t.Fatal("audit schema contains credential material")
	}
}

func TestPlatformTokenCoreConcurrentEnsureAndExpiredReplacement(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	s := platformCore(t, db, nil)
	var wg sync.WaitGroup
	results := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.EnsurePlatformToken(t.Context(), "owner") }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal("serialized bootstrap failed")
		}
	}
	migrationCounts(t, db, 0, 1)
	platformAuditCount(t, db, 1)
	r := repo.NewRepo(db.Gorm)
	old, err := r.GetCurrent(t.Context(), "owner")
	if err != nil {
		t.Fatal("current custody missing")
	}
	if err := db.Gorm.Exec("UPDATE media_auth.platform_tokens SET expires_at=? WHERE id=?", time.Now().Add(-time.Hour), old.ID).Error; err != nil {
		t.Fatal("fixture expiry failed")
	}
	if err := s.EnsurePlatformToken(t.Context(), "owner"); err != nil {
		t.Fatal("expired replacement failed")
	}
	current, err := r.GetCurrent(t.Context(), "owner")
	if err != nil || current.ID == old.ID || !current.ActiveAt(time.Now()) {
		t.Fatal("expired custody was not replaced")
	}
	old, err = r.GetIncludingRevoked(t.Context(), old.ID)
	if err != nil || old.RevokedAt == nil || old.ExpiresAt == nil {
		t.Fatal("expired history discarded")
	}
	migrationCounts(t, db, 0, 2)
	platformAuditCount(t, db, 2)
}

type failingPlatformAudit struct{ delegate domain.AuditWriter }

func (a failingPlatformAudit) AppendAudit(ctx context.Context, e domain.AuditEvent) error {
	if err := a.delegate.AppendAudit(ctx, e); err != nil {
		return err
	}
	return errors.New("private audit diagnostic")
}

func TestPlatformTokenCoreAuditFailureAndCorruptCustodyNeverImplicitlyIssue(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	r := repo.NewRepo(db.Gorm)
	broken := platformCore(t, db, failingPlatformAudit{r})
	if err := broken.EnsurePlatformToken(t.Context(), "owner"); err != service.ErrUnavailable {
		t.Fatal("audit error did not fail closed safely")
	}
	migrationCounts(t, db, 0, 0)
	platformAuditCount(t, db, 0)
	healthy := platformCore(t, db, nil)
	if err := healthy.EnsurePlatformToken(t.Context(), "owner"); err != nil {
		t.Fatal("healthy bootstrap failed")
	}
	old, err := r.GetCurrent(t.Context(), "owner")
	if err != nil {
		t.Fatal("current custody missing")
	}
	if secret, err := broken.RotatePlatformToken(t.Context(), "owner", domain.PurposeOffline, nil); secret != "" || err != service.ErrUnavailable {
		t.Fatal("rotation released uncommitted Secret")
	}
	if current, err := r.GetCurrent(t.Context(), "owner"); err != nil || current.ID != old.ID {
		t.Fatal("failed rotation destroyed current custody")
	}
	migrationCounts(t, db, 0, 1)
	platformAuditCount(t, db, 1)
	if secret, err := broken.AcquirePlatformToken(t.Context(), "owner", domain.PurposeOffline); secret != "" || err != service.ErrUnavailable {
		t.Fatal("acquire released Secret without audit commit")
	}
	if err := db.Gorm.Exec("UPDATE media_auth.platform_tokens SET key_version='unavailable',expires_at=? WHERE id=?", time.Now().Add(-time.Hour), old.ID).Error; err != nil {
		t.Fatal("fixture corruption failed")
	}
	if err := healthy.EnsurePlatformToken(t.Context(), "owner"); err != service.ErrUnavailable {
		t.Fatal("expired unreadable custody was silently replaced")
	}
	migrationCounts(t, db, 0, 1)
	platformAuditCount(t, db, 1)
	if value, err := healthy.RotatePlatformToken(t.Context(), "owner", domain.PurposeOffline, nil); err != nil || value == "" {
		t.Fatal("explicit recovery rotation failed")
	}
	migrationCounts(t, db, 0, 2)
	platformAuditCount(t, db, 2)
	if err := db.Gorm.Exec("UPDATE media_auth.platform_token_audit_outbox SET operation='arbitrary' WHERE user_id='owner'").Error; err == nil {
		t.Fatal("audit accepted arbitrary payload-like operation")
	}
	if err := db.Gorm.Exec("UPDATE media_auth.platform_token_audit_outbox SET lease_id='lease-without-deadline' WHERE user_id='owner'").Error; err == nil {
		t.Fatal("partial lease accepted")
	}
	if err := db.Gorm.Exec("UPDATE media_auth.platform_token_audit_outbox SET attempts=-1 WHERE user_id='owner'").Error; err == nil {
		t.Fatal("negative audit attempts accepted")
	}
	down, err := os.ReadFile("../migrations/0016_platform_token_audit.down.sql")
	if err != nil {
		t.Fatal("audit down migration missing")
	}
	sqlDB, err := db.Gorm.DB()
	if err != nil {
		t.Fatal("test DB unavailable")
	}
	conn, err := sqlDB.Conn(t.Context())
	if err != nil {
		t.Fatal("test connection unavailable")
	}
	defer conn.Close()
	if _, err := conn.ExecContext(t.Context(), string(down)); err == nil {
		t.Fatal("audit history discarded by downgrade")
	}
	if _, err := conn.ExecContext(t.Context(), "ROLLBACK"); err != nil {
		t.Fatal("downgrade guard cleanup failed")
	}
	platformAuditCount(t, db, 2)
}

type waitingPlatformUser struct {
	delegate         service.UserReader
	entered, release chan struct{}
}

func (u waitingPlatformUser) PlatformUserActive(ctx context.Context, id string, lock bool) (bool, error) {
	close(u.entered)
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-u.release:
	}
	return u.delegate.PlatformUserActive(ctx, id, lock)
}

func TestPlatformTokenCoreIntrospectionRechecksAfterConcurrentRotation(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	s := platformCore(t, db, nil)
	old, err := s.AcquirePlatformToken(t.Context(), "owner", domain.PurposeOffline)
	if err != nil {
		t.Fatal("acquire fixture failed")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	r := repo.NewRepo(db.Gorm)
	reader := waitingPlatformUser{authrepo.NewPlatformUserReader(db.Gorm), entered, release}
	stale := service.NewService(r, r, reader, dal.NewTxRunner(db.Gorm), platformTestRing(t), platformCaller{}, service.Options{})
	type outcome struct {
		p   service.Principal
		err error
	}
	result := make(chan outcome, 1)
	go func() {
		p, err := stale.IntrospectPlatformToken(ctx, old, domain.PurposeOffline)
		result <- outcome{p, err}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("introspection did not reach user lock")
	}
	_, err = s.RotatePlatformToken(ctx, "owner", domain.PurposeOffline, nil)
	close(release)
	if err != nil {
		t.Fatal("concurrent rotation failed")
	}
	select {
	case got := <-result:
		if got.err != nil || got.p.Active {
			t.Fatal("late introspection accepted rotated credential")
		}
	case <-ctx.Done():
		t.Fatal("late introspection did not resolve")
	}
}
