//go:build e2e

package e2e

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"media_agent/media_auth/biz/dal"
	apiRepository "media_agent/media_auth/biz/dal/repo/api_token"
	workerRepository "media_agent/media_auth/biz/dal/repo/worker_identity"
	api "media_agent/media_auth/biz/domain/api_token"
	worker "media_agent/media_auth/biz/domain/worker_identity"
	apiService "media_agent/media_auth/biz/service/api_token"
	workerService "media_agent/media_auth/biz/service/worker_identity"
	"media_agent/media_auth/biz/shared/client/talos"
	"media_agent/media_auth/biz/shared/ids"
	"media_agent/media_auth/biz/workflow"
)

func TestWorkerIdentityTalosCredentialRevocationChain(t *testing.T) {
	addr := os.Getenv("MEDIA_E2E_TALOS_URL")
	if addr == "" {
		t.Skip("MEDIA_E2E_TALOS_URL not set; real Worker credential cleanup not tested")
	}
	client, err := talos.New(talos.Config{BaseURL: addr, AdminToken: os.Getenv("MEDIA_E2E_TALOS_ADMIN_TOKEN")})
	if err != nil {
		t.Fatal(err)
	}
	db := newDB(t)
	owner := ids.New()
	seedUser(t, db, owner, "worker-owner")
	if err := db.Gorm.Exec(`INSERT INTO media_auth.organizations(id,name) VALUES ('org','Organization')`).Error; err != nil {
		t.Fatal(err)
	}
	workerRepo := workerRepository.NewRepo(db.Gorm)
	identity, err := workerRepo.Create(t.Context(), worker.Identity{ID: ids.New(), OwnerUserID: owner, OrganizationID: "org", Name: "worker-e2e", Status: worker.StatusActive, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	workerSvc := workerService.NewService(workerRepo)
	tokenRepo := apiRepository.NewRepo(db.Gorm)
	tx := dal.NewTxRunner(db.Gorm)
	adapter := apiService.TalosAdapter{Client: client}
	svc := apiService.NewService(tokenRepo, adapter)
	svc.SetIssuer(workflow.NewAPITokenIssuance(tokenRepo, adapter, tx))
	svc.SetSubjectValidator(func(_ context.Context, user string) bool { return user == owner })
	svc.SetMembershipValidator(func(_ context.Context, org, user string) bool { return org == "org" && user == owner })
	svc.SetActorValidator(func(ctx context.Context, kind api.ActorType, actor, user, org string) bool {
		return kind == api.ActorPersonalWorker && workerSvc.ValidateActor(ctx, actor, user, org)
	})
	result, err := svc.Create(t.Context(), apiService.CreateCommand{CreatorUserID: owner, SubjectUserID: owner,
		ActorType: api.ActorPersonalWorker, ActorID: identity.ActorID(), OrganizationID: "org", Name: "worker-e2e",
		Scopes: []string{"workflow.worker.poll"}, ExpiresAt: time.Now().Add(time.Hour), TrustedActorCreation: true})
	if err != nil {
		t.Fatal("Worker token issuance failed")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.Revoke(ctx, result.Token.TalosKeyID)
	})
	principal, err := svc.Verify(t.Context(), result.Secret)
	if err != nil || principal.ActorID != identity.ActorID() || principal.SubjectUserID != owner || principal.CredentialID != result.Token.ID {
		t.Fatal("Worker principal mismatch")
	}
	// Talos is still authoritative-active, but local identity must already deny
	// delegation immediately after the lifecycle transaction, before cleanup.
	flow := workflow.NewWorkerAccessRevocation(workerSvc, workerRepo, tokenRepo, tx)
	if err := flow.ChangeStatus(t.Context(), owner, identity.ID, worker.StatusDisabled); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Verify(t.Context(), result.Secret); err != nil {
		t.Fatal("test did not exercise pre-cleanup window")
	}
	if _, err := svc.Verify(t.Context(), result.Secret); !errors.Is(err, apiService.ErrInactive) {
		t.Fatal("disabled Worker credential admitted")
	}
	// Reinstantiate from durable facts, as after a process restart.
	restarted := apiService.NewService(apiRepository.NewRepo(db.Gorm), adapter)
	if _, err := workflow.NewAPITokenRevocation(tokenRepo, restarted).ProcessBatch(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	loaded, err := tokenRepo.Get(t.Context(), result.Token.ID)
	if err != nil || loaded.Status != api.StatusRevoked {
		t.Fatal("Worker key cleanup did not reach terminal state")
	}
	metadata, err := client.Get(t.Context(), result.Token.TalosKeyID)
	if err != nil || metadata.Status != "KEY_STATUS_REVOKED" {
		t.Fatal("Talos Worker key remained usable")
	}
	if _, err := client.Verify(t.Context(), result.Secret); err == nil {
		t.Fatal("revoked Worker secret verified")
	}
}
