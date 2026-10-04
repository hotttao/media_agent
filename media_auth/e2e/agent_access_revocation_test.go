//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"media_agent/media_auth/biz/dal"
	repository "media_agent/media_auth/biz/dal/repo/api_token"
	api "media_agent/media_auth/biz/domain/api_token"
	service "media_agent/media_auth/biz/service/api_token"
	"media_agent/media_auth/biz/shared/client/control"
	"media_agent/media_auth/biz/shared/client/talos"
	"media_agent/media_auth/biz/shared/ids"
	"media_agent/media_auth/biz/workflow"
)

type reconciliationAuthority struct {
	facts control.AgentIdentity
	err   error
}

func (a *reconciliationAuthority) ResolveAgentIdentity(context.Context, string) (control.AgentIdentity, error) {
	return a.facts, a.err
}

func TestAgentAccessRevocationPostgresScanAndDurableIntents(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	seedUser(t, db, "other", "other")
	if err := db.Gorm.Exec(`INSERT INTO media_auth.organizations(id,name) VALUES ('org','Organization'),('other','Other')`).Error; err != nil {
		t.Fatal(err)
	}
	repo := repository.NewRepo(db.Gorm)
	states := []api.Status{api.StatusActive, api.StatusRotating, api.StatusExpired, api.StatusIssuing, api.StatusRevoking, api.StatusIssuanceCleanup, api.StatusRevoked}
	for i, status := range states {
		draft := issuingDraft("owner")
		draft.ID, draft.ActorType, draft.ActorID, draft.OrganizationID, draft.Status = fmt.Sprintf("a-%d", i), api.ActorAgent, "Agent:stable", "org", status
		if status != api.StatusIssuing && status != api.StatusIssuanceCleanup {
			draft.TalosKeyID = "key-" + draft.ID
		}
		if _, err := repo.Create(t.Context(), draft); err != nil {
			t.Fatal(err)
		}
	}
	// Same actor suffix with a different kind must never enter Agent scans.
	worker := issuingDraft("owner")
	worker.ID, worker.ActorType, worker.ActorID, worker.OrganizationID = "z-worker", api.ActorPersonalWorker, "Worker:stable", "org"
	if _, err := repo.Create(t.Context(), worker); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListAgentRevocationCandidates(t.Context(), "", 1)
	if err != nil || len(rows) != 1 || rows[0].ID != "a-0" {
		t.Fatal("first candidate page incorrect", err)
	}
	if err := repo.PrepareActorRevocation(t.Context(), api.ActorAgent, "Agent:stable", "other", "org"); err != nil {
		t.Fatal(err)
	}
	if err := repo.PrepareActorRevocation(t.Context(), api.ActorAgent, "Agent:stable", "owner", "other"); err != nil {
		t.Fatal(err)
	}
	rows, err = repo.ListAgentRevocationCandidates(t.Context(), "a-0", 100)
	if err != nil || len(rows) != 3 || rows[0].ID != "a-1" || rows[2].ID != "a-3" {
		t.Fatal("scan lost statuses, cursor or identity boundary", err)
	}
	authority := &reconciliationAuthority{err: control.ErrUnavailable}
	assessor := service.NewAgentActorValidator(authority)
	flow := workflow.NewAgentAccessRevocation(repo, assessor, repo, dal.NewTxRunner(db.Gorm))
	if _, err := flow.ProcessBatch(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	rows, err = repo.ListAgentRevocationCandidates(t.Context(), "", 0)
	if err != nil || len(rows) != 4 {
		t.Fatal("authority outage changed credentials", err)
	}
	authority.err = control.ErrNotFound
	if _, err := flow.ProcessBatch(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	for i, original := range states {
		got, err := repo.Get(t.Context(), fmt.Sprintf("a-%d", i))
		want := api.StatusRevoking
		if original == api.StatusIssuing || original == api.StatusIssuanceCleanup {
			want = api.StatusIssuanceCleanup
		} else if original == api.StatusRevoked {
			want = api.StatusRevoked
		}
		if err != nil || got.Status != want {
			t.Fatal("durable cleanup state incorrect", err)
		}
	}
	untouched, err := repo.Get(t.Context(), worker.ID)
	if err != nil || untouched.Status != api.StatusIssuing {
		t.Fatal("Agent cleanup touched Worker", err)
	}
	// A late issuance is caught by a new process, while persisted cleanup stays.
	late := issuingDraft("owner")
	late.ActorType, late.ActorID, late.OrganizationID = api.ActorAgent, "Agent:stable", "org"
	if _, err := repo.Create(t.Context(), late); err != nil {
		t.Fatal(err)
	}
	restarted := repository.NewRepo(db.Gorm)
	if _, err := workflow.NewAgentAccessRevocation(restarted, assessor, restarted, dal.NewTxRunner(db.Gorm)).ProcessBatch(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.CommitIssuance(t.Context(), late.ID, "late-key", late.ExpiresAt, time.Now()); !errors.Is(err, api.ErrConflict) {
		t.Fatal("late activation resurrected revoked delegation")
	}
}

func TestAgentAccessRevocationTalosRecoveryAfterRestart(t *testing.T) {
	addr := os.Getenv("MEDIA_E2E_TALOS_URL")
	if addr == "" {
		t.Skip("MEDIA_E2E_TALOS_URL not set; real Agent credential cleanup not tested")
	}
	client, err := talos.New(talos.Config{BaseURL: addr, AdminToken: os.Getenv("MEDIA_E2E_TALOS_ADMIN_TOKEN")})
	if err != nil {
		t.Fatal(err)
	}
	db := newDB(t)
	owner, configuration := ids.New(), ids.New()
	seedUser(t, db, owner, "agent-owner")
	if err := db.Gorm.Exec(`INSERT INTO media_auth.organizations(id,name) VALUES ('org','Organization')`).Error; err != nil {
		t.Fatal(err)
	}
	repo := repository.NewRepo(db.Gorm)
	tx := dal.NewTxRunner(db.Gorm)
	authority := &reconciliationAuthority{facts: control.AgentIdentity{ConfigurationID: configuration, ActorID: "Agent:" + configuration,
		OwnerUserID: owner, OrganizationID: "org", WorkspaceID: "workspace", TeamID: "team", WorkspaceState: "stopped", OwnerAuthorized: true}}
	assessor := service.NewAgentActorValidator(authority)
	adapter := service.TalosAdapter{Client: client}
	svc := service.NewService(repo, adapter)
	svc.SetIssuer(workflow.NewAPITokenIssuance(repo, adapter, tx))
	svc.SetSubjectValidator(func(_ context.Context, user string) bool { return user == owner })
	svc.SetMembershipValidator(func(_ context.Context, org, user string) bool { return org == "org" && user == owner })
	svc.SetActorValidator(func(ctx context.Context, kind api.ActorType, actor, user, org string) bool {
		return kind == api.ActorAgent && assessor.Validate(ctx, actor, user, org)
	})
	result, err := svc.Create(t.Context(), service.CreateCommand{CreatorUserID: owner, SubjectUserID: owner, ActorType: api.ActorAgent,
		ActorID: authority.facts.ActorID, OrganizationID: "org", Name: "agent-reconciliation-e2e", Scopes: []string{"agent.gateway"},
		ExpiresAt: time.Now().Add(time.Hour), TrustedActorCreation: true})
	if err != nil {
		t.Fatal("test Agent token issuance failed")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.Revoke(ctx, result.Token.TalosKeyID)
	})
	if _, err := svc.Verify(t.Context(), result.Secret); err != nil {
		t.Fatal("test Agent token not initially valid")
	}
	authority.facts.OwnerAuthorized = false
	if _, err := workflow.NewAgentAccessRevocation(repo, assessor, repo, tx).ProcessBatch(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Verify(t.Context(), result.Secret); err != nil {
		t.Fatal("test did not exercise pre-cleanup window")
	}
	if _, err := svc.Verify(t.Context(), result.Secret); !errors.Is(err, service.ErrInactive) {
		t.Fatal("invalid Agent delegation admitted")
	}
	authority.facts.OwnerAuthorized = true // a committed revocation must not reactivate
	restarted := repository.NewRepo(db.Gorm)
	if _, err := workflow.NewAPITokenRevocation(restarted, service.NewService(restarted, adapter)).ProcessBatch(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	got, err := restarted.Get(t.Context(), result.Token.ID)
	if err != nil || got.Status != api.StatusRevoked {
		t.Fatal("restart lost revocation intent", err)
	}
	metadata, err := client.Get(t.Context(), result.Token.TalosKeyID)
	if err != nil || metadata.Status != "KEY_STATUS_REVOKED" {
		t.Fatal("Talos credential remained active")
	}
	if _, err := client.Verify(t.Context(), result.Secret); err == nil {
		t.Fatal("revoked Agent secret still verifies")
	}
}
