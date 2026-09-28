//go:build e2e

package e2e

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	repository "media_agent/media_auth/biz/dal/repo/api_token"
	domain "media_agent/media_auth/biz/domain/api_token"
	service "media_agent/media_auth/biz/service/api_token"
	"media_agent/media_auth/biz/shared/client/talos"
	"media_agent/media_auth/biz/workflow"
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

func TestAPITokenRepoRecoverySurvivesRestartAndLostResponse(t *testing.T) {
	repo, token := newAPITokenRepo(t)
	if _, err := repo.MarkRevoking(t.Context(), token.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	var revoked atomic.Bool
	var attempt atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2alpha1/admin/issuedApiKeys/old-key:revoke":
			if attempt.Add(1) >= 2 {
				revoked.Store(true)
			}
			w.WriteHeader(http.StatusServiceUnavailable) // success response lost on the second call
		case r.Method == http.MethodGet && r.URL.Path == "/v2alpha1/admin/issuedApiKeys/old-key":
			status := "KEY_STATUS_ACTIVE"
			if revoked.Load() {
				status = "KEY_STATUS_REVOKED"
			}
			_, _ = w.Write([]byte(`{"key_id":"old-key","status":"` + status + `"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	client, err := talos.New(talos.Config{BaseURL: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}
	newRecovery := func() *workflow.APITokenRevocation {
		// New process objects reuse only durable metadata, never request memory.
		return workflow.NewAPITokenRevocation(repo, service.NewService(repo, service.TalosAdapter{Client: client}))
	}
	if _, err := newRecovery().ProcessBatch(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.Get(t.Context(), token.ID)
	if err != nil || pending.Status != domain.StatusRevoking || pending.ActiveAt(time.Now()) {
		t.Fatal("upstream failure must remain fail closed")
	}
	if _, err := newRecovery().ProcessBatch(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	completed, err := repo.Get(t.Context(), token.ID)
	if err != nil || completed.Status != domain.StatusRevoked || completed.RevokedAt == nil {
		t.Fatal("restart did not reconcile authoritative revoked status")
	}
	if _, err := newRecovery().ProcessBatch(t.Context(), ""); err != nil || attempt.Load() != 2 {
		t.Fatal("terminal token was replayed")
	}
}

func TestAPITokenRepoRecoveryScanUsesStableKeyset(t *testing.T) {
	repo, token := newAPITokenRepo(t)
	if _, err := repo.MarkRevoking(t.Context(), token.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	second := token
	second.ID, second.TalosKeyID = "z-pending", "second-key"
	second.Status = domain.StatusRevoking
	if _, err := repo.Create(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListRevoking(t.Context(), "", 1)
	if err != nil || len(rows) != 1 || rows[0].ID != token.ID {
		t.Fatal("first page incorrect")
	}
	if err := repo.MarkRevoked(t.Context(), token.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	rows, err = repo.ListRevoking(t.Context(), token.ID, 1)
	if err != nil || len(rows) != 1 || rows[0].ID != second.ID {
		t.Fatal("completed previous page skipped pending token")
	}
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
