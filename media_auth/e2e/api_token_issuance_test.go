//go:build e2e

package e2e

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"media_agent/media_auth/biz/dal"
	repository "media_agent/media_auth/biz/dal/repo/api_token"
	domain "media_agent/media_auth/biz/domain/api_token"
	service "media_agent/media_auth/biz/service/api_token"
	"media_agent/media_auth/biz/shared/client/talos"
	"media_agent/media_auth/biz/shared/ids"
	"media_agent/media_auth/biz/workflow"
)

type droppedIssuanceResponse struct {
	base         http.RoundTripper
	dropped      atomic.Bool
	blockCleanup atomic.Bool
}

func (d *droppedIssuanceResponse) RoundTrip(r *http.Request) (*http.Response, error) {
	if d.dropped.Load() && d.blockCleanup.Load() {
		return nil, errors.New("simulated cleanup unavailable")
	}
	response, err := d.base.RoundTrip(r)
	if err == nil && r.Method == http.MethodPost && r.URL.Path == "/v2alpha1/admin/issuedApiKeys" &&
		response.StatusCode >= 200 && response.StatusCode < 300 && d.dropped.CompareAndSwap(false, true) {
		// Talos has committed; do not read, log, or retain the one-time Secret.
		_ = response.Body.Close()
		return nil, io.ErrUnexpectedEOF
	}
	return response, err
}

func TestAPITokenIssuanceRealTalosLostResponseSurvivesRestart(t *testing.T) {
	addr := os.Getenv("MEDIA_E2E_TALOS_URL")
	if addr == "" {
		t.Skip("MEDIA_E2E_TALOS_URL not set; durable real Talos recovery not tested")
	}
	db := newDB(t)
	owner := ids.New()
	seedUser(t, db, owner, "issuance-owner")
	repo := repository.NewRepo(db.Gorm)
	tx := dal.NewTxRunner(db.Gorm)
	token := issuingDraft(owner)
	client, err := talos.New(talos.Config{BaseURL: addr, AdminToken: os.Getenv("MEDIA_E2E_TALOS_ADMIN_TOKEN")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		page, err := client.List(ctx, token.ActorID, "")
		if err != nil {
			t.Error("test key cleanup enumeration failed")
			return
		}
		for _, key := range page.Keys {
			if key.Metadata["api_token_id"] == token.ID && key.Status != "KEY_STATUS_REVOKED" {
				if err := client.Revoke(ctx, key.KeyID); err != nil {
					t.Error("test key cleanup failed")
				}
			}
		}
	})
	transport := &droppedIssuanceResponse{base: http.DefaultTransport}
	transport.blockCleanup.Store(true)
	faulty, err := talos.New(talos.Config{BaseURL: addr, AdminToken: os.Getenv("MEDIA_E2E_TALOS_ADMIN_TOKEN"), Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	flow := workflow.NewAPITokenIssuance(repo, service.TalosAdapter{Client: faulty}, tx)
	result, err := flow.Issue(t.Context(), token)
	if err == nil || result.Secret != "" || !transport.dropped.Load() {
		t.Fatal("Talos committed-response loss not exercised")
	}
	loaded, err := repo.Get(t.Context(), token.ID)
	if err != nil || loaded.Status != domain.StatusIssuanceCleanup || loaded.TalosKeyID != "" || loaded.ActiveAt(time.Now()) {
		t.Fatal("unknown key lost durable cleanup intent")
	}
	// Only persisted metadata is provided to the restarted workflow.
	restarted := workflow.NewAPITokenIssuance(repository.NewRepo(db.Gorm), service.TalosAdapter{Client: client}, tx)
	if _, err := restarted.ProcessBatch(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	loaded, err = repo.Get(t.Context(), token.ID)
	if err != nil || loaded.Status != domain.StatusRevoked || loaded.TalosKeyID == "" {
		t.Fatal("restart did not finish cleanup")
	}
	page, err := client.List(t.Context(), token.ActorID, "")
	if err != nil || len(page.Keys) != 1 || page.Keys[0].KeyID != loaded.TalosKeyID || page.Keys[0].Status != "KEY_STATUS_REVOKED" {
		t.Fatal("restart left an unknown live key")
	}
	if result, err := client.Issue(t.Context(), talos.IssueCommand{Name: token.Name, ActorID: token.ActorID, RequestID: token.ID, TTL: "1h", Scopes: token.Scopes,
		Metadata: map[string]any{"api_token_id": token.ID, "issuance_id": token.ID}}); err == nil || result.Secret != "" {
		t.Fatal("late request delivered a replacement credential")
	}
	if _, err := restarted.ProcessBatch(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	page, err = client.List(t.Context(), token.ActorID, "")
	if err != nil || len(page.Keys) != 1 || page.Keys[0].Status != "KEY_STATUS_REVOKED" {
		t.Fatal("terminal cleanup or late issue recreated a key")
	}
}

// This proves the pinned upstream contract, not durable Workflow recovery.
// The latter additionally requires persisted intent and local CAS tests.
func TestAPITokenIssuanceTalosRecoveryContract(t *testing.T) {
	addr := os.Getenv("MEDIA_E2E_TALOS_URL")
	if addr == "" {
		t.Skip("MEDIA_E2E_TALOS_URL not set; real Talos issuance recovery not tested")
	}
	for _, loseOriginal := range []bool{true, false} {
		name := "recover before original arrives"
		if loseOriginal {
			name = "committed original response lost"
		}
		t.Run(name, func(t *testing.T) {
			client, err := talos.New(talos.Config{BaseURL: addr, AdminToken: os.Getenv("MEDIA_E2E_TALOS_ADMIN_TOKEN")})
			if err != nil {
				t.Fatal(err)
			}
			attempt := ids.New()
			actor := "User:issuance-e2e-" + ids.New()
			cmd := talos.IssueCommand{Name: "issuance-recovery-e2e", ActorID: actor, RequestID: attempt, TTL: "1h",
				Scopes: []string{"api.access"}, Metadata: map[string]any{"api_token_id": attempt, "issuance_id": attempt}}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				page, err := client.List(ctx, actor, "")
				if err != nil {
					t.Error("could not enumerate temporary issuance credentials for cleanup")
					return
				}
				for _, key := range page.Keys {
					if key.Metadata["api_token_id"] == attempt && key.Status != "KEY_STATUS_REVOKED" {
						if err := client.Revoke(ctx, key.KeyID); err != nil {
							t.Error("could not revoke temporary issuance credential")
						}
					}
				}
			})
			if loseOriginal {
				transport := &droppedIssuanceResponse{base: http.DefaultTransport}
				faulty, err := talos.New(talos.Config{BaseURL: addr, AdminToken: os.Getenv("MEDIA_E2E_TALOS_ADMIN_TOKEN"), Transport: transport})
				if err != nil {
					t.Fatal(err)
				}
				result, err := faulty.Issue(t.Context(), cmd)
				if !errors.Is(err, talos.ErrUnavailable) || result.Secret != "" || !transport.dropped.Load() {
					t.Fatal("real Talos committed-response loss not exercised")
				}
			}
			// New client simulates lost in-memory response. It uses only stable
			// request identity; no recovered Secret is available to the caller.
			key, err := client.RecoverIssue(t.Context(), cmd)
			if err != nil || key.KeyID == "" || key.ActorID != actor {
				t.Fatalf("real Talos issuance metadata recovery failed: %v", err)
			}
			if err := client.Revoke(t.Context(), key.KeyID); err != nil {
				t.Fatal(err)
			}
			// Replaying even after revocation must resolve the same terminal key,
			// never create a second credential or reactivate the old one.
			replayed, err := client.RecoverIssue(t.Context(), cmd)
			if err != nil || replayed.KeyID != key.KeyID || replayed.Status != "KEY_STATUS_REVOKED" {
				t.Fatal("replay did not preserve revoked issuance identity")
			}
			if result, err := client.Issue(t.Context(), cmd); err == nil || result.Secret != "" {
				t.Fatal("late normal Issue replay delivered a credential")
			}
			page, err := client.List(t.Context(), actor, "")
			if err != nil || page.NextPageToken != "" || len(page.Keys) != 1 ||
				page.Keys[0].KeyID != key.KeyID || page.Keys[0].Status != "KEY_STATUS_REVOKED" {
				t.Fatal("recovery or late replay created an additional live key")
			}
		})
	}
}
