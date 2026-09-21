package virtual_ip

import (
	"context"
	"testing"

	domain "media_agent/media_materials/biz/domain/virtual_ip"
)

type fakeRepo struct{ item *domain.VirtualIp }

func (f *fakeRepo) Save(_ context.Context, v *domain.VirtualIp) error           { f.item = v; return nil }
func (f *fakeRepo) Get(_ context.Context, id string) (*domain.VirtualIp, error) { return f.item, nil }
func (f *fakeRepo) List(_ context.Context, q domain.ListQuery) ([]*domain.VirtualIp, int, error) {
	if f.item == nil {
		return nil, 0, nil
	}
	return []*domain.VirtualIp{f.item}, 1, nil
}
func (f *fakeRepo) Update(_ context.Context, v *domain.VirtualIp) error { f.item = v; return nil }
func (f *fakeRepo) Delete(context.Context, string) error                { return nil }

type fakeGrants struct{}

func (fakeGrants) AddGrant(context.Context, *domain.Grant) error               { return nil }
func (fakeGrants) RemoveGrant(context.Context, string, string) error           { return nil }
func (fakeGrants) ListGrants(context.Context, string) ([]*domain.Grant, error) { return nil, nil }
func (fakeGrants) ListByTeam(context.Context, string) ([]*domain.Grant, error) { return nil, nil }

func TestCreateVirtualIPPersistsPersonaAndObjectPaths(t *testing.T) {
	s := NewService(&fakeRepo{}, fakeGrants{})
	v, err := s.Create(context.Background(), CreateCommand{OrganizationID: "org1", Scope: "ai_clothing", Name: "Ada",
		AvatarObjectPath: "virtual-ips/ada/avatar.png", PersonaProfile: `{"age":28}`, OwnerUserID: "u1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if v.OrganizationID != "org1" || v.AvatarObjectPath == "" || v.PersonaProfile != `{"age":28}` {
		t.Fatalf("unexpected virtual ip: %+v", v)
	}
}

func TestCreateVirtualIPRejectsInvalidPersonaJSON(t *testing.T) {
	s := NewService(&fakeRepo{}, fakeGrants{})
	_, err := s.Create(context.Background(), CreateCommand{OrganizationID: "org1", Scope: "ai_clothing", Name: "Ada", PersonaProfile: `{`, OwnerUserID: "u1"})
	if err == nil {
		t.Fatal("expected invalid persona JSON")
	}
}

func TestUpdateVirtualIPRejectsOrganizationMismatch(t *testing.T) {
	r := &fakeRepo{item: &domain.VirtualIp{ID: "v1", OrganizationID: "org1", OwnerUserID: "u1"}}
	_, err := NewService(r, fakeGrants{}).Update(context.Background(), UpdateCommand{ID: "v1", OrganizationID: "org2", CallerID: "u1"})
	if err == nil {
		t.Fatal("expected organization mismatch")
	}
}

func TestArchiveVirtualIPRejectsOrganizationMismatchBeforeMutation(t *testing.T) {
	r := &fakeRepo{item: &domain.VirtualIp{ID: "v1", OrganizationID: "org1", OwnerUserID: "u1", Status: domain.StatusActive}}
	_, err := NewService(r, fakeGrants{}).Archive(context.Background(), "v1", "org2", "u1")
	if err == nil {
		t.Fatal("expected organization mismatch")
	}
	if r.item.Status != domain.StatusActive {
		t.Fatalf("archive mutated cross-organization resource: %s", r.item.Status)
	}
}

func TestArchiveVirtualIPRequiresOwner(t *testing.T) {
	r := &fakeRepo{item: &domain.VirtualIp{ID: "v1", OrganizationID: "org1", OwnerUserID: "u1", Status: domain.StatusActive}}
	_, err := NewService(r, fakeGrants{}).Archive(context.Background(), "v1", "org1", "u2")
	if err == nil {
		t.Fatal("expected owner authorization error")
	}
	if r.item.Status != domain.StatusActive {
		t.Fatalf("archive mutated non-owner resource: %s", r.item.Status)
	}
}
