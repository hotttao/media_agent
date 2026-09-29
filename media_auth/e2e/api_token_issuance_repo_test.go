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
	"media_agent/media_auth/biz/dal/model"
	repository "media_agent/media_auth/biz/dal/repo/api_token"
	domain "media_agent/media_auth/biz/domain/api_token"
	"media_agent/media_auth/biz/shared/ids"
)

func issuingDraft(owner string) domain.Token {
	now := time.Now()
	expiry, deadline := now.Add(time.Hour), now.Add(time.Minute)
	return domain.Token{ID: ids.New(), SubjectUserID: owner, ActorType: domain.ActorUser, ActorID: "User:" + owner,
		Name: "issuance-e2e", Scopes: []string{"api.access"}, Status: domain.StatusIssuing, CreatedBy: owner,
		CreatedAt: now, ExpiresAt: &expiry, IssuanceDeadline: &deadline}
}

func TestAPITokenIssuanceRepoNullableKeyAndConcurrentRequestDeduplication(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	seedUser(t, db, "other", "other")
	repo := repository.NewRepo(db.Gorm)
	token := issuingDraft("owner")
	token.ClientRequestKey = "same-client-key"
	var workers sync.WaitGroup
	outcomes := make(chan error, 2)
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			attempt := token
			attempt.ID = ids.New()
			_, err := repo.Create(t.Context(), attempt)
			outcomes <- err
		}()
	}
	workers.Wait()
	close(outcomes)
	winners := 0
	for err := range outcomes {
		if err == nil {
			winners++
		} else if !errors.Is(err, domain.ErrConflict) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatal("concurrent duplicate creation accepted more than once")
	}
	other := issuingDraft("other")
	other.ClientRequestKey = token.ClientRequestKey
	if _, err := repo.Create(t.Context(), other); err != nil {
		t.Fatal("request deduplication leaked across creators")
	}
	for i := 0; i < 2; i++ {
		if _, err := repo.Create(t.Context(), issuingDraft("owner")); err != nil {
			t.Fatal("NULL keys or absent request IDs collided")
		}
	}
	var rows []model.APIToken
	if err := db.Gorm.Find(&rows).Error; err != nil || len(rows) != 4 {
		t.Fatal("prospective token row count incorrect")
	}
	for _, row := range rows {
		if row.TalosKeyID != nil || row.IssuanceDeadline == nil {
			t.Fatal("unsigned key was not NULL or durable deadline lost")
		}
	}
}

func TestAPITokenIssuanceRepoRecoveryFencesLateActivationAndRevoke(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	seedUser(t, db, "other", "other")
	repo := repository.NewRepo(db.Gorm)
	token, err := repo.Create(t.Context(), issuingDraft("owner"))
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := repo.ClaimIssuanceCleanup(t.Context(), token.ID, time.Now(), false); err != nil || claimed {
		t.Fatal("background claimed live issuance")
	}
	if _, err := repo.MarkRevoking(t.Context(), token.ID, "other"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("other owner revoked pending issuance")
	}
	pending, err := repo.MarkRevoking(t.Context(), token.ID, "owner")
	if err != nil || pending.Status != domain.StatusIssuanceCleanup || pending.TalosKeyID != "" {
		t.Fatal("pending issuance used ordinary revocation")
	}
	if _, err := repo.CommitIssuance(t.Context(), token.ID, "late-key", token.ExpiresAt, time.Now()); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("late result reactivated cleanup intent")
	}
	if err := repo.MarkRevoked(t.Context(), token.ID, time.Now()); err == nil {
		t.Fatal("ordinary revocation discarded issuance intent")
	}
	if err := repo.FinishIssuanceCleanup(t.Context(), token.ID, "", time.Now()); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("unknown key marked cleaned")
	}
	if err := repo.FinishIssuanceCleanup(t.Context(), token.ID, "cleaned-key", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := repo.FinishIssuanceCleanup(t.Context(), token.ID, "cleaned-key", time.Now()); err != nil {
		t.Fatal("same cleanup not idempotent")
	}
	if err := repo.FinishIssuanceCleanup(t.Context(), token.ID, "other-key", time.Now()); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("terminal key overwritten")
	}
	loaded, err := repo.Get(t.Context(), token.ID)
	if err != nil || loaded.Status != domain.StatusRevoked || loaded.TalosKeyID != "cleaned-key" || loaded.RevokedAt == nil {
		t.Fatal("terminal cleanup facts incomplete")
	}
}

func TestAPITokenIssuanceRepoActivationIsAtomicAndCompletedNotRecovered(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	repo := repository.NewRepo(db.Gorm)
	tx := dal.NewTxRunner(db.Gorm)
	token := issuingDraft("owner")
	token.ClientRequestKey = "once"
	token, err := repo.Create(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.RunInTx(t.Context(), func(ctx context.Context) error {
		if _, err := repo.CommitIssuance(ctx, token.ID, "key", token.ExpiresAt, time.Now()); err != nil {
			return err
		}
		return errors.New("simulated transaction failure")
	})
	if err == nil {
		t.Fatal("failed transaction accepted")
	}
	loaded, err := repo.Get(t.Context(), token.ID)
	if err != nil || loaded.Status != domain.StatusIssuing || loaded.TalosKeyID != "" {
		t.Fatal("failed transaction left partially active credential")
	}
	if _, err := repo.CommitIssuance(t.Context(), token.ID, "key", token.ExpiresAt, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := repo.ClaimIssuanceCleanup(t.Context(), token.ID, time.Now(), true); err != nil || claimed {
		t.Fatal("completed issuance compensated")
	}
	rows, err := repo.ListPendingIssuances(t.Context(), "", time.Now().Add(time.Hour), 100)
	if err != nil || len(rows) != 0 {
		t.Fatal("completed token still scanned")
	}
	retry := issuingDraft("owner")
	retry.ClientRequestKey = token.ClientRequestKey
	if _, err := repo.Create(t.Context(), retry); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("active request duplicated")
	}
	if _, err := repo.MarkRevoking(t.Context(), token.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkRevoked(t.Context(), token.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(t.Context(), retry); err != nil {
		t.Fatal("revoked request prevented new replacement attempt")
	}
}

func TestAPITokenIssuanceRepoDeadlineScanAndRollbackGuard(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	repo := repository.NewRepo(db.Gorm)
	token := issuingDraft("owner")
	deadline := time.Now().Add(-time.Second)
	token.IssuanceDeadline = &deadline
	token, err := repo.Create(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CommitIssuance(t.Context(), token.ID, "late-key", token.ExpiresAt, time.Now()); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("expired issuance activated")
	}
	rows, err := repo.ListPendingIssuances(t.Context(), "", time.Now(), 1)
	if err != nil || len(rows) != 1 || rows[0].ID != token.ID {
		t.Fatal("expired intent not scanned")
	}
	rows, err = repo.ListPendingIssuances(t.Context(), token.ID, time.Now(), 1)
	if err != nil || len(rows) != 0 {
		t.Fatal("cursor did not exclude prior page")
	}
	if _, claimed, err := repo.ClaimIssuanceCleanup(t.Context(), token.ID, time.Now(), false); err != nil || !claimed {
		t.Fatal("expired intent not claimable")
	}
	rollback, err := os.ReadFile("../migrations/0013_api_token_issuance.down.sql")
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
		t.Fatal("rollback discarded pending issuance")
	}
	if _, err := conn.ExecContext(t.Context(), "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.Get(t.Context(), token.ID)
	if err != nil || loaded.Status != domain.StatusIssuanceCleanup {
		t.Fatal("rollback guard altered cleanup intent")
	}
}
