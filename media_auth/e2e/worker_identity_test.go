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
	apiRepository "media_agent/media_auth/biz/dal/repo/api_token"
	repository "media_agent/media_auth/biz/dal/repo/worker_identity"
	api "media_agent/media_auth/biz/domain/api_token"
	domain "media_agent/media_auth/biz/domain/worker_identity"
	service "media_agent/media_auth/biz/service/worker_identity"
	"media_agent/media_auth/biz/shared/ids"
	"media_agent/media_auth/biz/workflow"
)

func TestWorkerIdentityPostgresOwnershipAndCurrentActorStatus(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	seedUser(t, db, "other", "other")
	if err := db.Gorm.Exec(`INSERT INTO media_auth.organizations(id,name) VALUES ('org','Organization'),('other','Other')`).Error; err != nil {
		t.Fatal(err)
	}
	repo := repository.NewRepo(db.Gorm)
	svc := service.NewService(repo)
	svc.SetUserValidator(func(_ context.Context, user string) bool { return user == "owner" || user == "other" })
	svc.SetMembershipValidator(func(_ context.Context, org, user string) bool {
		return org == "org" && (user == "owner" || user == "other")
	})
	identity, err := svc.Create(t.Context(), service.CreateCommand{OwnerUserID: "owner", OrganizationID: "org", Name: "  桌面 Worker  "})
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.Create(t.Context(), service.CreateCommand{OwnerUserID: "other", OrganizationID: "org", Name: "Other worker"})
	if err != nil {
		t.Fatal(err)
	}
	own, err := svc.List(t.Context(), "owner")
	if err != nil || len(own) != 1 || own[0].ID != identity.ID || own[0].Name != "桌面 Worker" {
		t.Fatal("owner query leaked identity or lost data", err)
	}
	if _, err := svc.GetOwned(t.Context(), "owner", other.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("other owner identity exposed")
	}
	if !svc.ValidateActor(t.Context(), identity.ActorID(), "owner", "org") || svc.ValidateActor(t.Context(), identity.ActorID(), "other", "org") || svc.ValidateActor(t.Context(), identity.ActorID(), "owner", "other") {
		t.Fatal("actor ownership or organization mismatch")
	}
	if err := svc.ChangeStatus(t.Context(), "owner", identity.ID, domain.StatusDisabled); err != nil {
		t.Fatal(err)
	}
	if svc.ValidateActor(t.Context(), identity.ActorID(), "owner", "org") {
		t.Fatal("disabled actor accepted")
	}
	disabled, err := repo.Get(t.Context(), identity.ID)
	if err != nil || disabled.DisabledAt == nil {
		t.Fatal("disable timestamp missing", err)
	}
	if err := svc.ChangeStatus(t.Context(), "owner", identity.ID, domain.StatusDisabled); err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangeStatus(t.Context(), "owner", identity.ID, domain.StatusActive); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("disabled actor resurrected")
	}
	if err := svc.ChangeStatus(t.Context(), "owner", identity.ID, domain.StatusDeleted); err != nil {
		t.Fatal(err)
	}
	deleted, err := repo.Get(t.Context(), identity.ID)
	if err != nil || deleted.DisabledAt == nil || deleted.DeletedAt == nil || !deleted.DisabledAt.Equal(*disabled.DisabledAt) {
		t.Fatal("deletion lost identity history", err)
	}
}

type failingActorCleanup struct{}

func (failingActorCleanup) PrepareActorRevocation(context.Context, api.ActorType, string, string, string) error {
	return errors.New("cleanup write unavailable")
}

func TestWorkerIdentityRevocationCommitsIntentsAndCatchesLateIssuance(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	if err := db.Gorm.Exec(`INSERT INTO media_auth.organizations(id,name) VALUES ('org','Organization'),('other','Other')`).Error; err != nil {
		t.Fatal(err)
	}
	repo := repository.NewRepo(db.Gorm)
	identity, err := repo.Create(t.Context(), domain.Identity{ID: ids.New(), OwnerUserID: "owner", OrganizationID: "org", Name: "worker", Status: domain.StatusActive, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewService(repo)
	tokenRepo := apiRepository.NewRepo(db.Gorm)
	tx := dal.NewTxRunner(db.Gorm)
	failing := workflow.NewWorkerAccessRevocation(svc, repo, failingActorCleanup{}, tx)
	if err := failing.ChangeStatus(t.Context(), "owner", identity.ID, domain.StatusDisabled); err == nil {
		t.Fatal("partial disable accepted")
	}
	unchanged, err := repo.Get(t.Context(), identity.ID)
	if err != nil || unchanged.Status != domain.StatusActive {
		t.Fatal("token write failure did not rollback identity", err)
	}
	states := []api.Status{api.StatusActive, api.StatusRotating, api.StatusExpired, api.StatusIssuing, api.StatusRevoked}
	tokens := []api.Token{}
	for _, status := range states {
		token := issuingDraft("owner")
		token.ActorType, token.ActorID, token.OrganizationID, token.Status = api.ActorPersonalWorker, identity.ActorID(), "org", status
		if status != api.StatusIssuing {
			token.TalosKeyID = "key-" + token.ID
		}
		created, err := tokenRepo.Create(t.Context(), token)
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, created)
	}
	// An unrelated Organization and Agent with the same suffix are not revoked.
	unrelated := issuingDraft("owner")
	unrelated.ActorType, unrelated.ActorID, unrelated.OrganizationID = api.ActorPersonalWorker, identity.ActorID(), "other"
	unrelated, err = tokenRepo.Create(t.Context(), unrelated)
	if err != nil {
		t.Fatal(err)
	}
	agent := issuingDraft("owner")
	agent.ActorType, agent.ActorID, agent.OrganizationID = api.ActorAgent, "Agent:"+identity.ID, "org"
	agent, err = tokenRepo.Create(t.Context(), agent)
	if err != nil {
		t.Fatal(err)
	}
	flow := workflow.NewWorkerAccessRevocation(svc, repo, tokenRepo, tx)
	if err := flow.ChangeStatus(t.Context(), "owner", identity.ID, domain.StatusDisabled); err != nil {
		t.Fatal(err)
	}
	for _, token := range tokens {
		loaded, err := tokenRepo.Get(t.Context(), token.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := api.StatusRevoking
		if token.Status == api.StatusIssuing {
			want = api.StatusIssuanceCleanup
		}
		if token.Status == api.StatusRevoked {
			want = api.StatusRevoked
		}
		if loaded.Status != want {
			t.Fatalf("%s -> %s, want %s", token.Status, loaded.Status, want)
		}
	}
	for _, token := range []api.Token{unrelated, agent} {
		loaded, err := tokenRepo.Get(t.Context(), token.ID)
		if err != nil || loaded.Status != api.StatusIssuing {
			t.Fatal("actor cleanup crossed identity boundary", err)
		}
	}
	// Simulate a creation that validated before disable but persisted afterwards.
	late := issuingDraft("owner")
	late.ActorType, late.ActorID, late.OrganizationID = api.ActorPersonalWorker, identity.ActorID(), "org"
	late, err = tokenRepo.Create(t.Context(), late)
	if err != nil {
		t.Fatal(err)
	}
	if cursor, err := flow.ProcessBatch(t.Context(), ""); err != nil || cursor != "" {
		t.Fatal(err)
	}
	loaded, err := tokenRepo.Get(t.Context(), late.ID)
	if err != nil || loaded.Status != api.StatusIssuanceCleanup {
		t.Fatal("late issuance escaped recovery", err)
	}
	if _, err := tokenRepo.CommitIssuance(t.Context(), late.ID, "late-key", late.ExpiresAt, time.Now()); !errors.Is(err, api.ErrConflict) {
		t.Fatal("late issuance restored active credential")
	}
	if err := flow.ChangeStatus(t.Context(), "owner", identity.ID, domain.StatusDisabled); err != nil {
		t.Fatal("repeat disable failed", err)
	}
}

func TestWorkerIdentityPostgresCASAndTransactionRollback(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	if err := db.Gorm.Exec(`INSERT INTO media_auth.organizations(id,name) VALUES ('org','Organization')`).Error; err != nil {
		t.Fatal(err)
	}
	repo := repository.NewRepo(db.Gorm)
	identity, err := repo.Create(t.Context(), domain.Identity{ID: ids.New(), OwnerUserID: "owner", OrganizationID: "org", Name: "worker", Status: domain.StatusActive, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("transaction failed after identity update")
	if err := dal.NewTxRunner(db.Gorm).RunInTx(t.Context(), func(ctx context.Context) error {
		if err := repo.UpdateStatus(ctx, identity.ID, "owner", domain.StatusActive, domain.StatusDisabled, time.Now()); err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	loaded, err := repo.Get(t.Context(), identity.ID)
	if err != nil || loaded.Status != domain.StatusActive || loaded.DisabledAt != nil {
		t.Fatal("rollback leaked status change", err)
	}
	if err := repo.UpdateStatus(t.Context(), identity.ID, "other", domain.StatusActive, domain.StatusDisabled, time.Now()); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("owner CAS missing")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, next := range []domain.Status{domain.StatusDisabled, domain.StatusDeleted} {
		wg.Add(1)
		go func(next domain.Status) {
			defer wg.Done()
			results <- repo.UpdateStatus(t.Context(), identity.ID, "owner", domain.StatusActive, next, time.Now())
		}(next)
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, domain.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("CAS wins=%d conflicts=%d", wins, conflicts)
	}
}

func TestWorkerIdentityMigrationRejectsInvalidRowsAndHistoryLoss(t *testing.T) {
	db := newDB(t)
	seedUser(t, db, "owner", "owner")
	if err := db.Gorm.Exec(`INSERT INTO media_auth.organizations(id,name) VALUES ('org','Organization')`).Error; err != nil {
		t.Fatal(err)
	}
	base := model.WorkerIdentity{ID: ids.New(), OwnerUserID: "owner", OrganizationID: "org", Name: "worker", Status: "active", CreatedAt: time.Now()}
	now := time.Now()
	for _, change := range []func(*model.WorkerIdentity){
		func(r *model.WorkerIdentity) { r.ID = "instance-name" },
		func(r *model.WorkerIdentity) { r.OwnerUserID = "missing" },
		func(r *model.WorkerIdentity) { r.OrganizationID = "missing" },
		func(r *model.WorkerIdentity) { r.Name = " " },
		func(r *model.WorkerIdentity) { r.Status = "unknown" },
		func(r *model.WorkerIdentity) { r.Status = "disabled" },
		func(r *model.WorkerIdentity) { r.Status = "deleted" },
		func(r *model.WorkerIdentity) { r.DisabledAt = &now },
	} {
		row := base
		row.ID = ids.New()
		change(&row)
		if err := db.Gorm.Create(&row).Error; err == nil {
			t.Fatal("invalid identity row accepted")
		}
	}
	if err := db.Gorm.Create(&base).Error; err != nil {
		t.Fatal(err)
	}
	rollback, err := os.ReadFile("../migrations/0014_worker_identities.down.sql")
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
		t.Fatal("rollback discarded stable identity")
	}
	if _, err := conn.ExecContext(t.Context(), "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewRepo(db.Gorm).Get(t.Context(), base.ID); err != nil {
		t.Fatal("rollback guard lost identity", err)
	}
}
