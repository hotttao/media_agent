//go:build e2e

package e2e

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	repository "media_agent/media_auth/biz/dal/repo/api_token"
	domain "media_agent/media_auth/biz/domain/api_token"
	service "media_agent/media_auth/biz/service/api_token"
	"media_agent/media_auth/biz/shared/client/talos"
	"media_agent/media_auth/biz/shared/ids"
	"media_agent/media_auth/biz/workflow"
)

func rotationFixture(t *testing.T) (*repository.Repo, domain.Token, dal.TxRunner, func(string) domain.RotationStatus) {
	t.Helper()
	db := newDB(t)
	ownerID := ids.New()
	seedUser(t, db, ownerID, "rotation-owner")
	repo := repository.NewRepo(db.Gorm)
	expiry := time.Now().Add(time.Hour)
	token, err := repo.Create(t.Context(), domain.Token{ID: ids.New(), TalosKeyID: "old-key", SubjectUserID: ownerID, ActorType: domain.ActorUser,
		ActorID: "User:" + ownerID, Name: "rotation-e2e", Scopes: []string{"api.access"}, Status: domain.StatusActive, ExpiresAt: &expiry, CreatedBy: ownerID, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	status := func(id string) domain.RotationStatus {
		var row model.APITokenRotation
		if err := db.Gorm.WithContext(t.Context()).First(&row, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		return domain.RotationStatus(row.Status)
	}
	return repo, token, dal.NewTxRunner(db.Gorm), status
}

func TestAPITokenRotationRepoConcurrentBeginAndRecoveryFence(t *testing.T) {
	repo, token, tx, status := rotationFixture(t)
	now := time.Now()
	operations := []domain.Rotation{
		{ID: "operation-1", TokenID: token.ID, OldKeyID: token.TalosKeyID, ActorID: token.ActorID, CreatedAt: now, Deadline: now.Add(time.Minute)},
		{ID: "operation-2", TokenID: token.ID, OldKeyID: token.TalosKeyID, ActorID: token.ActorID, CreatedAt: now, Deadline: now.Add(time.Minute)},
	}
	type outcome struct {
		op  domain.Rotation
		err error
	}
	outcomes := make(chan outcome, 2)
	var workers sync.WaitGroup
	for _, op := range operations {
		workers.Add(1)
		go func(op domain.Rotation) {
			defer workers.Done()
			err := tx.RunInTx(t.Context(), func(ctx context.Context) error { return repo.BeginRotation(ctx, token, op) })
			outcomes <- outcome{op, err}
		}(op)
	}
	workers.Wait()
	close(outcomes)
	var winner domain.Rotation
	winners := 0
	for out := range outcomes {
		if out.err == nil {
			winner = out.op
			winners++
		} else if !errors.Is(out.err, domain.ErrConflict) {
			t.Fatal(out.err)
		}
	}
	if winners != 1 {
		t.Fatal("concurrent requests both began rotation")
	}
	if claimed, err := repo.ClaimRotationRecovery(t.Context(), winner.ID, now, false); err != nil || claimed {
		t.Fatal("recovery claimed live request")
	}
	if claimed, err := repo.ClaimRotationRecovery(t.Context(), winner.ID, now, true); err != nil || !claimed {
		t.Fatal("recovery failed to fence late response")
	}
	if claimed, err := repo.ClaimRotationRecovery(t.Context(), "unrelated-operation", now, false); err != nil || claimed {
		t.Fatal("recovery claim escaped operation ID scope")
	}
	err := tx.RunInTx(t.Context(), func(ctx context.Context) error {
		_, err := repo.CommitRotation(ctx, winner, "late-key", token.ExpiresAt, now)
		return err
	})
	if !errors.Is(err, domain.ErrConflict) || status(winner.ID) != domain.RotationRecovering {
		t.Fatal("late commit escaped recovery fence")
	}
	if err := tx.RunInTx(t.Context(), func(ctx context.Context) error { return repo.AbortRotation(ctx, winner, now) }); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.Get(t.Context(), token.ID)
	if err != nil || loaded.Status != domain.StatusRevoked || status(winner.ID) != domain.RotationAborted {
		t.Fatal("cleanup terminal facts not atomic")
	}
}

func TestAPITokenRotationRepoConcurrentRevokeRollsBackCompletedOperation(t *testing.T) {
	repo, token, tx, status := rotationFixture(t)
	now := time.Now()
	op := domain.Rotation{ID: "operation", TokenID: token.ID, OldKeyID: token.TalosKeyID, ActorID: token.ActorID, CreatedAt: now, Deadline: now.Add(time.Minute)}
	if err := tx.RunInTx(t.Context(), func(ctx context.Context) error { return repo.BeginRotation(ctx, token, op) }); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkRevoking(t.Context(), token.ID, token.SubjectUserID); err != nil {
		t.Fatal(err)
	}
	err := tx.RunInTx(t.Context(), func(ctx context.Context) error {
		_, err := repo.CommitRotation(ctx, op, "new-key", token.ExpiresAt, now)
		return err
	})
	if !errors.Is(err, domain.ErrConflict) || status(op.ID) != domain.RotationPending {
		t.Fatal("failed token commit left falsely completed operation")
	}
	rows, err := repo.ListPendingRotations(t.Context(), "", op.Deadline.Add(time.Second), 100)
	if err != nil || len(rows) != 1 || rows[0].ID != op.ID {
		t.Fatal("pending operation lost after concurrent revoke")
	}
}

func TestAPITokenRotationRepoSuccessfulCommitPreservesMetadata(t *testing.T) {
	repo, token, tx, status := rotationFixture(t)
	now := time.Now()
	op := domain.Rotation{ID: "successful-operation", TokenID: token.ID, OldKeyID: token.TalosKeyID, ActorID: token.ActorID, CreatedAt: now, Deadline: now.Add(time.Minute)}
	if err := tx.RunInTx(t.Context(), func(ctx context.Context) error { return repo.BeginRotation(ctx, token, op) }); err != nil {
		t.Fatal(err)
	}
	var rotated domain.Token
	err := tx.RunInTx(t.Context(), func(ctx context.Context) error {
		var err error
		rotated, err = repo.CommitRotation(ctx, op, "new-key", token.ExpiresAt, now)
		return err
	})
	if err != nil || rotated.ID != token.ID || rotated.ActorID != token.ActorID || rotated.SubjectUserID != token.SubjectUserID || rotated.TalosKeyID != "new-key" || !rotated.ActiveAt(now) || status(op.ID) != domain.RotationCompleted {
		t.Fatal("rotation lost metadata or failed to complete")
	}
	if _, err := repo.GetByTalosKeyID(t.Context(), token.TalosKeyID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("old key mapping survived rotation")
	}
	if claimed, err := repo.ClaimRotationRecovery(t.Context(), op.ID, now.Add(time.Hour), true); err != nil || claimed {
		t.Fatal("completed operation was claimable")
	}
	rows, err := repo.ListPendingRotations(t.Context(), "", now.Add(time.Hour), 100)
	if err != nil || len(rows) != 0 {
		t.Fatal("completed operation scanned for recovery")
	}
}

func TestAPITokenRotationRepoRejectsExpiredTokenBeforeSideEffects(t *testing.T) {
	repo, token, tx, _ := rotationFixture(t)
	at := token.ExpiresAt.Add(time.Second)
	op := domain.Rotation{ID: "expired-operation", TokenID: token.ID, OldKeyID: token.TalosKeyID, ActorID: token.ActorID, CreatedAt: at, Deadline: at.Add(time.Minute)}
	err := tx.RunInTx(t.Context(), func(ctx context.Context) error { return repo.BeginRotation(ctx, token, op) })
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatal("expired token began rotation")
	}
}

type droppedRotationResponse struct {
	base         http.RoundTripper
	dropped      atomic.Bool
	blockCleanup atomic.Bool
}

func (d *droppedRotationResponse) RoundTrip(r *http.Request) (*http.Response, error) {
	if d.dropped.Load() && d.blockCleanup.Load() {
		return nil, errors.New("simulated cleanup unavailable")
	}
	response, err := d.base.RoundTrip(r)
	if err == nil && strings.HasSuffix(r.URL.Path, ":rotate") && response.StatusCode >= 200 && response.StatusCode < 300 && !d.dropped.Swap(true) {
		// Talos really commits rotation; discard its one-time Secret without
		// logging or persisting it, then simulate a lost response.
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		return nil, errors.New("simulated lost rotation response")
	}
	return response, err
}

func TestAPITokenRotationRealTalosLostResponseCleanup(t *testing.T) {
	addr := os.Getenv("MEDIA_E2E_TALOS_URL")
	if addr == "" {
		t.Skip("set MEDIA_E2E_TALOS_URL for real Talos lost-response integration")
	}
	repo, token, tx, status := rotationFixture(t)
	client, err := talos.New(talos.Config{BaseURL: addr, AdminToken: os.Getenv("MEDIA_E2E_TALOS_ADMIN_TOKEN")})
	if err != nil {
		t.Fatal(err)
	}
	// The fixture uses a unique User/Actor; never touch existing service keys.
	issued, err := client.Issue(t.Context(), talos.IssueCommand{Name: "rotation-recovery-e2e", ActorID: token.ActorID, TTL: "1h", Scopes: token.Scopes, Metadata: map[string]any{"api_token_id": token.ID}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		page, err := client.List(ctx, token.ActorID, "")
		if err == nil {
			for _, key := range page.Keys {
				_ = client.Revoke(ctx, key.KeyID)
			}
		}
	})
	// Fixture-only metadata replacement before the actual workflow starts.
	if _, err := repo.ReplaceTalosKey(t.Context(), token.ID, token.TalosKeyID, issued.KeyID, token.ExpiresAt, time.Now()); err != nil {
		t.Fatal(err)
	}
	token.TalosKeyID = issued.KeyID
	transport := &droppedRotationResponse{base: http.DefaultTransport}
	transport.blockCleanup.Store(true)
	faulty, err := talos.New(talos.Config{BaseURL: addr, AdminToken: os.Getenv("MEDIA_E2E_TALOS_ADMIN_TOKEN"), Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	flow := workflow.NewAPITokenRotation(repo, service.TalosAdapter{Client: faulty}, tx)
	result, err := flow.Rotate(t.Context(), token)
	if err == nil || result.Secret != "" || !transport.dropped.Load() {
		t.Fatalf("real Talos loss was not exercised safely: response_dropped=%v error=%v", transport.dropped.Load(), err)
	}
	loaded, err := repo.Get(t.Context(), token.ID)
	if err != nil || loaded.Status != domain.StatusRotating || loaded.ActiveAt(time.Now()) {
		t.Fatal("failed cleanup did not preserve fail-closed intent")
	}
	// Restart with only persisted metadata and a healthy connection, not the
	// replacement Secret or the original request's memory.
	restarted := workflow.NewAPITokenRotation(repo, service.TalosAdapter{Client: client}, tx)
	if _, err := restarted.ProcessBatch(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	loaded, err = repo.Get(t.Context(), token.ID)
	if err != nil || loaded.Status != domain.StatusRevoked {
		t.Fatal("restart failed to clean unknown replacement")
	}
	page, err := client.List(t.Context(), token.ActorID, "")
	if err != nil {
		t.Fatal(err)
	}
	replacements := 0
	for _, key := range page.Keys {
		if key.Metadata["api_token_id"] != token.ID {
			continue
		}
		if key.Status != "KEY_STATUS_REVOKED" {
			t.Fatal("rotation left a live Talos key")
		}
		if operationID, ok := key.Metadata["rotation_id"].(string); ok && operationID != "" {
			replacements++
			if status(operationID) != domain.RotationAborted {
				t.Fatal("cleanup was not durably completed")
			}
		}
	}
	if replacements != 1 {
		t.Fatal("cleanup missed replacement or rotated more than once")
	}
	// A late direct rotation cannot create another key after the old-key fence.
	if _, err := client.Rotate(t.Context(), token.TalosKeyID, token.Scopes); err == nil {
		t.Fatal("Talos accepted rotation after recovery fence")
	}
}
