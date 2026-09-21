package organization

import (
	"context"
	"testing"

	domain "media_agent/media_auth/biz/domain/organization"
)

type fakeRepo struct {
	members       map[string]domain.Membership
	admins        int
	organizations []domain.Summary
}

func (f *fakeRepo) ListForUser(context.Context, string) ([]domain.Summary, error) {
	return f.organizations, nil
}
func (f *fakeRepo) Create(context.Context, string, string) (domain.Summary, error) {
	return domain.Summary{}, nil
}
func (f *fakeRepo) Get(context.Context, string) (domain.Organization, error) {
	return domain.Organization{}, nil
}
func (f *fakeRepo) Update(context.Context, string, *string, *string) (domain.Organization, error) {
	return domain.Organization{}, nil
}
func (f *fakeRepo) GetMembership(_ context.Context, org, user string) (domain.Membership, error) {
	m, ok := f.members[user]
	if !ok {
		return domain.Membership{}, domain.ErrNotMember
	}
	return m, nil
}
func (f *fakeRepo) ListMembers(context.Context, string) ([]domain.Membership, error) { return nil, nil }
func (f *fakeRepo) PutMember(_ context.Context, org, user, role string) (domain.Membership, error) {
	m := domain.Membership{OrganizationID: org, UserID: user, Role: role}
	f.members[user] = m
	return m, nil
}
func (f *fakeRepo) RemoveMember(_ context.Context, org, user string) error {
	delete(f.members, user)
	return nil
}
func (f *fakeRepo) CountAdmins(context.Context, string) (int, error)          { return f.admins, nil }
func (f *fakeRepo) UserExists(context.Context, string) (bool, error)          { return true, nil }
func (f *fakeRepo) GetEntitlements(context.Context, string) ([]string, error) { return nil, nil }
func (f *fakeRepo) SetEntitlements(_ context.Context, _, _ string, keys []string) ([]string, error) {
	return keys, nil
}

func TestCreateRequiresPlatformAdmin(t *testing.T) {
	s := NewService(&fakeRepo{})
	if _, err := s.Create(context.Background(), "u", "user", "Acme"); err != ErrPlatformAdmin {
		t.Fatalf("err=%v", err)
	}
}
func TestCannotDemoteLastOrganizationAdmin(t *testing.T) {
	r := &fakeRepo{members: map[string]domain.Membership{"actor": {UserID: "actor", Role: "admin"}}, admins: 1}
	s := NewService(r)
	if _, err := s.PutMember(context.Background(), "actor", "org", "actor", "member"); err != ErrLastAdmin {
		t.Fatalf("err=%v", err)
	}
}
func TestSetEntitlementsRequiresBothPlatformAndOrganizationAdmin(t *testing.T) {
	r := &fakeRepo{members: map[string]domain.Membership{"actor": {UserID: "actor", Role: "member"}}}
	s := NewService(r)
	if _, err := s.SetEntitlements(context.Background(), "actor", "admin", "org", []string{"workflow.run"}); err != ErrForbidden {
		t.Fatalf("err=%v", err)
	}
}

func TestListActiveIDsExcludesDisabledOrganizations(t *testing.T) {
	r := &fakeRepo{organizations: []domain.Summary{{Organization: domain.Organization{ID: "org1", Status: "active"}}, {Organization: domain.Organization{ID: "org2", Status: "disabled"}}}}
	ids, err := NewService(r).ListActiveIDs(context.Background(), "u1")
	if err != nil || len(ids) != 1 || ids[0] != "org1" {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
}
