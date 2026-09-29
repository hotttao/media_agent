package workeridentity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domain "media_agent/media_auth/biz/domain/worker_identity"
)

type fakeRepo struct {
	identity      domain.Identity
	err           error
	reads, writes int
}

func (r *fakeRepo) Create(_ context.Context, i domain.Identity) (domain.Identity, error) {
	r.writes++
	r.identity = i
	return i, r.err
}
func (r *fakeRepo) Get(_ context.Context, id string) (domain.Identity, error) {
	r.reads++
	if r.err != nil {
		return domain.Identity{}, r.err
	}
	if r.identity.ID != id {
		return domain.Identity{}, domain.ErrNotFound
	}
	return r.identity, nil
}
func (r *fakeRepo) ListByOwner(_ context.Context, owner string) ([]domain.Identity, error) {
	if r.identity.OwnerUserID != owner {
		return []domain.Identity{}, nil
	}
	return []domain.Identity{r.identity}, r.err
}
func (r *fakeRepo) UpdateStatus(_ context.Context, id, owner string, old, next domain.Status, at time.Time) error {
	r.writes++
	if r.err != nil {
		return r.err
	}
	if r.identity.ID != id || r.identity.OwnerUserID != owner || r.identity.Status != old {
		return domain.ErrConflict
	}
	r.identity.Status = next
	if next == domain.StatusDisabled {
		r.identity.DisabledAt = &at
	}
	if next == domain.StatusDeleted {
		r.identity.DeletedAt = &at
	}
	return nil
}

func TestCreateRequiresActiveUserAndOrganizationAndGeneratesIdentity(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	cmd := CreateCommand{OwnerUserID: "owner", OrganizationID: "org", Name: "  本地客户端  "}
	if _, err := svc.Create(t.Context(), cmd); !errors.Is(err, domain.ErrForbidden) || repo.writes != 0 {
		t.Fatal("missing authority accepted")
	}
	svc.SetUserValidator(func(_ context.Context, user string) bool { return user == "owner" })
	svc.SetMembershipValidator(func(_ context.Context, org, user string) bool { return org == "org" && user == "owner" })
	for _, invalid := range []CreateCommand{
		{OwnerUserID: "other", OrganizationID: "org", Name: "x"},
		{OwnerUserID: "owner", OrganizationID: "other", Name: "x"},
		{OwnerUserID: "owner", OrganizationID: "org", Name: "  "},
		{OwnerUserID: "owner", OrganizationID: "org", Name: strings.Repeat("中", 201)},
		{OwnerUserID: "owner", OrganizationID: "org", Name: string([]byte{0xff})},
	} {
		if _, err := svc.Create(t.Context(), invalid); err == nil || repo.writes != 0 {
			t.Fatal("invalid or unauthorized creation reached storage")
		}
	}
	identity, err := svc.Create(t.Context(), cmd)
	if err != nil || len(identity.ID) != 32 || identity.ActorID() != "Worker:"+identity.ID || identity.Name != "本地客户端" || identity.Status != domain.StatusActive || identity.CreatedAt.IsZero() {
		t.Fatalf("Create = %+v, %v", identity, err)
	}
	repo.err = errors.New("database offline")
	if _, err := svc.Create(t.Context(), cmd); !errors.Is(err, repo.err) {
		t.Fatal("write error lost")
	}
}

func TestActorValidationUsesCurrentOwnerOrganizationAndStatus(t *testing.T) {
	repo := &fakeRepo{identity: domain.Identity{ID: "stable", OwnerUserID: "owner", OrganizationID: "org", Status: domain.StatusActive}}
	svc := NewService(repo)
	for _, tc := range []struct {
		actor, owner, org string
		want              bool
	}{
		{"Worker:stable", "owner", "org", true},
		{"Worker:stable", "other", "org", false},
		{"Worker:stable", "owner", "other", false},
		{"Worker:stable", "owner", "", false},
		{"Worker:stable ", "owner", "org", false},
		{"Worker:", "owner", "org", false},
		{"Agent:stable", "owner", "org", false},
		{"stable", "owner", "org", false},
		{"Worker:instance", "owner", "org", false},
	} {
		if got := svc.ValidateActor(t.Context(), tc.actor, tc.owner, tc.org); got != tc.want {
			t.Fatalf("validation %+v = %v", tc, got)
		}
	}
	repo.identity.Status = domain.StatusDisabled
	if svc.ValidateActor(t.Context(), "Worker:stable", "owner", "org") {
		t.Fatal("cached active identity")
	}
	repo.identity.Status = domain.StatusActive
	repo.err = errors.New("storage offline")
	if svc.ValidateActor(t.Context(), "Worker:stable", "owner", "org") {
		t.Fatal("authority error did not fail closed")
	}
}

func TestLifecycleHidesOtherOwnersAndNeverReactivates(t *testing.T) {
	repo := &fakeRepo{identity: domain.Identity{ID: "stable", OwnerUserID: "owner", OrganizationID: "org", Status: domain.StatusActive}}
	svc := NewService(repo)
	if _, err := svc.GetOwned(t.Context(), "other", "stable"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("other owner identity leaked")
	}
	if err := svc.ChangeStatus(t.Context(), "other", "stable", domain.StatusDisabled); !errors.Is(err, domain.ErrNotFound) || repo.writes != 0 {
		t.Fatal("other owner changed status")
	}
	if err := svc.ChangeStatus(t.Context(), "owner", "stable", domain.StatusDisabled); err != nil || repo.identity.DisabledAt == nil {
		t.Fatal("disable failed", err)
	}
	writes := repo.writes
	if err := svc.ChangeStatus(t.Context(), "owner", "stable", domain.StatusDisabled); err != nil || repo.writes != writes {
		t.Fatal("repeated disable not idempotent")
	}
	if err := svc.ChangeStatus(t.Context(), "owner", "stable", domain.StatusActive); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("reactivation allowed")
	}
	if err := svc.ChangeStatus(t.Context(), "owner", "stable", domain.StatusDeleted); err != nil || repo.identity.DeletedAt == nil {
		t.Fatal("delete failed", err)
	}
	if err := svc.ChangeStatus(t.Context(), "owner", "stable", domain.StatusActive); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("resurrection allowed")
	}
}
