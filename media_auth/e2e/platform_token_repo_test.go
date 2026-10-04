//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"media_agent/media_auth/biz/dal"
	repository "media_agent/media_auth/biz/dal/repo/platform_token"
	domain "media_agent/media_auth/biz/domain/platform_token"
	"media_agent/media_auth/biz/shared/platformcipher"
)

func platformDraft(t *testing.T, ring *platformcipher.Keyring, id, user string, seed int) domain.Token {
	t.Helper()
	e, err := ring.Seal(id, user, fmt.Sprintf("tos_%064x", seed))
	if err != nil {
		t.Fatal("test encryption failed")
	}
	return domain.Token{ID: id, UserID: user, KeyVersion: e.KeyVersion, TokenHash: e.TokenHash, Ciphertext: e.Ciphertext, Nonce: e.Nonce, CreatedAt: time.Now().UTC()}
}

func platformTestRing(t *testing.T) *platformcipher.Keyring {
	t.Helper()
	key := make([]byte, 32)
	key[0] = 1
	ring, err := platformcipher.New("test-v1", map[string][]byte{"test-v1": key})
	if err != nil {
		t.Fatal(err)
	}
	return ring
}

func TestPlatformTokenPostgresIndependentEncryptedCustodyAndConcurrentCurrentUniqueness(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	seedUser(t, db, "other", "other")
	repo := repository.NewRepo(db.Gorm)
	ring := platformTestRing(t)
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for i := 0; i < 2; i++ {
		draft := platformDraft(t, ring, fmt.Sprintf("token-%d", i), "owner", i+1)
		wg.Add(1)
		go func() { defer wg.Done(); outcomes <- repo.Create(t.Context(), draft) }()
	}
	wg.Wait()
	close(outcomes)
	winners, conflicts := 0, 0
	for err := range outcomes {
		if err == nil {
			winners++
		} else if errors.Is(err, domain.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatal("multiple current credentials were committed")
	}
	current, err := repo.GetCurrent(t.Context(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := ring.Open(current.ID, current.UserID, platformcipher.Envelope{KeyVersion: current.KeyVersion, TokenHash: current.TokenHash, Ciphertext: current.Ciphertext, Nonce: current.Nonce})
	if err != nil || secret == "" {
		t.Fatal("persisted encrypted custody cannot be recovered")
	}
	byHash, err := repo.GetByHash(t.Context(), current.TokenHash)
	if err != nil || byHash.ID != current.ID {
		t.Fatal("custody hash lookup failed")
	}
	if _, err := repo.GetCurrent(t.Context(), "other"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("owner lookup leaked custody")
	}
	other := platformDraft(t, ring, "other-token", "other", 10)
	if err := repo.Create(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	// Expired current rows still occupy the slot until explicitly revoked.
	expiry := time.Now().Add(-time.Hour)
	if err := db.Gorm.Exec("UPDATE media_auth.platform_tokens SET expires_at=? WHERE id=?", expiry, current.ID).Error; err != nil {
		t.Fatal(err)
	}
	current, err = repo.GetCurrent(t.Context(), "owner")
	if err != nil || current.ActiveAt(time.Now()) {
		t.Fatal("expiry was ignored or current record hidden")
	}
	if err := repo.Create(t.Context(), platformDraft(t, ring, "replacement", "owner", 20)); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("expired custody was replaced without revocation")
	}
	at := time.Now().UTC()
	if err := repo.RevokeForUser(t.Context(), "owner", at); err != nil {
		t.Fatal(err)
	}
	if err := repo.RevokeForUser(t.Context(), "owner", at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByHash(t.Context(), current.TokenHash); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("revoked hash remained active")
	}
	if _, err := repo.GetCurrent(t.Context(), "other"); err != nil {
		t.Fatal("revocation crossed User boundary")
	}
	if err := repo.Create(t.Context(), platformDraft(t, ring, "replacement", "owner", 20)); err != nil {
		t.Fatal("revoked slot could not be replaced")
	}
	var old struct{ RevokedAt time.Time }
	if err := db.Gorm.Table("media_auth.platform_tokens").Select("revoked_at").Where("id=?", current.ID).Take(&old).Error; err != nil || old.RevokedAt.Sub(at).Abs() > time.Microsecond {
		t.Fatal("revoked history or timestamp changed")
	}
	var plaintextColumns int
	if err := db.Gorm.Raw("SELECT count(*) FROM information_schema.columns WHERE table_schema='media_auth' AND table_name='platform_tokens' AND column_name IN ('token','plaintext','secret')").Scan(&plaintextColumns).Error; err != nil || plaintextColumns != 0 {
		t.Fatal("independent table contains plaintext columns")
	}
}

func TestPlatformTokenPostgresRollbackAndMigrationHistoryGuard(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	repo := repository.NewRepo(db.Gorm)
	ring := platformTestRing(t)
	original := platformDraft(t, ring, "original", "owner", 1)
	if err := repo.Create(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	abort := errors.New("abort test transaction")
	err := dal.NewTxRunner(db.Gorm).RunInTx(t.Context(), func(ctx context.Context) error {
		if err := repo.RevokeForUser(ctx, "owner", time.Now()); err != nil {
			return err
		}
		if err := repo.Create(ctx, platformDraft(t, ring, "replacement", "owner", 2)); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatal("transaction failure lost")
	}
	got, err := repo.GetCurrent(t.Context(), "owner")
	if err != nil || got.ID != original.ID {
		t.Fatal("rollback did not retain original custody")
	}
	if err := db.Gorm.Exec("UPDATE media_auth.platform_tokens SET nonce=? WHERE id=?", []byte{1}, original.ID).Error; err == nil {
		t.Fatal("invalid nonce shape accepted")
	}
	if err := db.Gorm.Exec("UPDATE media_auth.platform_tokens SET token_hash=? WHERE id=?", []byte{1}, original.ID).Error; err == nil {
		t.Fatal("invalid hash shape accepted")
	}
	rollback, err := os.ReadFile("../migrations/0015_platform_tokens.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.Gorm.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := sqlDB.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(t.Context(), string(rollback)); err == nil {
		t.Fatal("migration discarded encrypted custody")
	}
	if _, err := conn.ExecContext(t.Context(), "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetCurrent(t.Context(), "owner"); err != nil {
		t.Fatal("history guard lost custody")
	}
}
