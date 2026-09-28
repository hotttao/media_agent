//go:build e2e

package e2e

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	repository "media_agent/media_auth/biz/dal/repo/api_token"
	domain "media_agent/media_auth/biz/domain/api_token"
)

func newAPITokenRepo(t *testing.T) (*repository.Repo, domain.Token) {
	t.Helper()
	db := newDB(t)
	seedUser(t, db, "owner", "token-owner")
	seedUser(t, db, "other", "other-user")
	repo := repository.NewRepo(db.Gorm)
	expires := time.Now().Add(time.Hour)
	token, err := repo.Create(t.Context(), domain.Token{ID: "api-test", TalosKeyID: "old-key", SubjectUserID: "owner",
		ActorType: domain.ActorUser, ActorID: "User:owner", Name: "e2e", Scopes: []string{"api.access"}, Status: domain.StatusActive,
		ExpiresAt: &expires, CreatedBy: "owner", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return repo, token
}

func TestAPITokenRepoRevocationRejectsWrongOwnerAndCannotBeResurrected(t *testing.T) {
	repo, token := newAPITokenRepo(t)
	ctx := t.Context()
	if _, err := repo.MarkRevoking(ctx, token.ID, "other"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("wrong owner: %v", err)
	}
	loaded, err := repo.GetByTalosKeyID(ctx, token.TalosKeyID)
	if err != nil || loaded.ActorID != token.ActorID || !loaded.ActiveAt(time.Now()) {
		t.Fatal("metadata roundtrip failed")
	}
	if _, err := repo.MarkRevoking(ctx, token.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	// A second request retries the durable revoking state.
	pending, err := repo.MarkRevoking(ctx, token.ID, "owner")
	if err != nil || pending.Status != domain.StatusRevoking || pending.ActiveAt(time.Now()) {
		t.Fatal("revocation retry not fail closed")
	}
	if _, err := repo.ReplaceTalosKey(ctx, token.ID, "old-key", "new-key", token.ExpiresAt, time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("rotation revived revoking token: %v", err)
	}
	if err := repo.MarkRevoked(ctx, token.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReplaceTalosKey(ctx, token.ID, "old-key", "new-key", token.ExpiresAt, time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("rotation revived revoked token: %v", err)
	}
	revoked, err := repo.MarkRevoking(ctx, token.ID, "owner")
	if err != nil || revoked.Status != domain.StatusRevoked {
		t.Fatal("repeat revoke not idempotent")
	}
}

func TestAPITokenRepoConcurrentRotationUsesCompareAndSwap(t *testing.T) {
	repo, token := newAPITokenRepo(t)
	ctx := context.Background()
	var workers sync.WaitGroup
	outcomes := make(chan error, 2)
	for _, key := range []string{"new-key-1", "new-key-2"} {
		workers.Add(1)
		go func(key string) {
			defer workers.Done()
			_, err := repo.ReplaceTalosKey(ctx, token.ID, token.TalosKeyID, key, token.ExpiresAt, time.Now())
			outcomes <- err
		}(key)
	}
	workers.Wait()
	close(outcomes)
	winners := 0
	for err := range outcomes {
		if err == nil {
			winners++
		} else if !errors.Is(err, domain.ErrNotFound) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatal("concurrent rotations both committed")
	}
	if _, err := repo.GetByTalosKeyID(ctx, token.TalosKeyID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("old key mapping survived rotation")
	}
	listed, err := repo.ListBySubject(ctx, "owner")
	if err != nil || len(listed) != 1 || listed[0].Status != domain.StatusActive {
		t.Fatal("token metadata not preserved")
	}
}
