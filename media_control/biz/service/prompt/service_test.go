package prompt

import (
	"context"
	"errors"
	"media_agent/hertz_infra/authn/resourceaccess"
	domain "media_agent/media_control/biz/domain/prompt"
	"testing"
	"time"
)

type fakeRepository struct {
	value   *domain.Prompt
	history map[int64]domain.Version
}

func (f *fakeRepository) Get(context.Context, string) (*domain.Prompt, error) {
	if f.value == nil {
		return nil, domain.ErrNotFound
	}
	v := *f.value
	v.Grants = append([]domain.Grant{}, v.Grants...)
	return &v, nil
}
func (f *fakeRepository) List(context.Context, string, string, []string) ([]domain.Prompt, error) {
	if f.value == nil {
		return nil, nil
	}
	return []domain.Prompt{*f.value}, nil
}
func (f *fakeRepository) Save(_ context.Context, v *domain.Prompt, expected int64) error {
	if expected != 0 && (f.value == nil || f.value.Version != expected) {
		return domain.ErrConflict
	}
	copy := *v
	f.value = &copy
	if f.history == nil {
		f.history = map[int64]domain.Version{}
	}
	f.history[v.Version] = domain.Version{Version: v.Version, Name: v.Name, Content: v.Content}
	return nil
}
func (f *fakeRepository) Versions(context.Context, string) ([]domain.Version, error) {
	out := []domain.Version{}
	for _, v := range f.history {
		out = append(out, v)
	}
	return out, nil
}
func (f *fakeRepository) GetVersion(_ context.Context, _ string, version int64) (*domain.Version, error) {
	v, ok := f.history[version]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &v, nil
}

type directTx struct{}

func (f *fakeRepository) ListUnassigned(context.Context, string) ([]domain.Prompt, error) {
	return nil, nil
}
func (f *fakeRepository) AssignOrganization(ctx context.Context, v *domain.Prompt, expected int64) error {
	return f.Save(ctx, v, expected)
}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fakeAccess struct{ unavailable bool }

func (a *fakeAccess) Resolve(_ context.Context, user, org string) (resourceaccess.Facts, error) {
	if a.unavailable {
		return resourceaccess.Facts{}, errors.New("auth offline")
	}
	return resourceaccess.Facts{Member: org == "org" && user != "outsider", TeamIDs: []string{user + "-team"}}, nil
}
func (a *fakeAccess) Team(_ context.Context, _ string, id string) (resourceaccess.Team, error) {
	org := "org"
	if id == "foreign-team" {
		org = "another-org"
	}
	return resourceaccess.Team{TeamID: id, OrganizationID: org, TeamActive: id != "disabled-team", OrganizationActive: true}, nil
}
func fixture() (*Service, *fakeRepository, *fakeAccess) {
	now := time.Now().UTC()
	r := &fakeRepository{value: &domain.Prompt{ID: "p", OrganizationID: "org", OwnerUserID: "owner", Name: "Prompt", Content: "old", Visibility: "team", Grants: []domain.Grant{{TeamID: "editor-team", Permission: "editor"}, {TeamID: "viewer-team", Permission: "viewer"}}, Status: domain.StatusActive, Version: 2, CreatedAt: now, UpdatedAt: now}, history: map[int64]domain.Version{1: {Version: 1, Content: "original"}}}
	a := &fakeAccess{}
	return New(r, directTx{}, a), r, a
}
func TestEditorsCanChangeContentButCannotChangeACLOrArchive(t *testing.T) {
	s, r, _ := fixture()
	ctx := context.Background()
	if _, err := s.Update(ctx, UpdateCommand{PromptID: "p", OwnerUserID: "editor", Name: "new", Content: "changed", ExpectedVersion: 1}); !errors.Is(err, domain.ErrConflict) {
		t.Fatal(err)
	}
	v, err := s.Update(ctx, UpdateCommand{PromptID: "p", OwnerUserID: "editor", Name: "new", Content: "changed", ExpectedVersion: 2})
	if err != nil || v.Version != 3 || v.MyPermission != "editor" {
		t.Fatalf("%+v %v", v, err)
	}
	if _, err := s.SetACL(ctx, ACLCommand{PromptID: "p", UserID: "editor", Visibility: "public", ExpectedVersion: 3}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.Archive(ctx, ArchiveCommand{PromptID: "p", OwnerUserID: "editor", ExpectedVersion: 3}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal(err)
	}
	if r.value.Content != "changed" || r.value.Visibility != "team" {
		t.Fatal("unauthorized write")
	}
}
func TestViewersCannotWriteAndPublicDoesNotCrossOrganizations(t *testing.T) {
	s, r, _ := fixture()
	ctx := context.Background()
	if v, err := s.Get(ctx, "p", "viewer"); err != nil || v.MyPermission != "viewer" {
		t.Fatal(v, err)
	}
	if _, err := s.Update(ctx, UpdateCommand{PromptID: "p", OwnerUserID: "viewer", Name: "x", Content: "x", ExpectedVersion: 2}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal(err)
	}
	r.value.Visibility = "public"
	if _, err := s.Get(ctx, "p", "outsider"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
	if values, err := s.List(ctx, "outsider", "org"); !errors.Is(err, domain.ErrNotFound) || len(values) != 0 {
		t.Fatal(values, err)
	}
}
func TestRevokedMembershipAndAuthFailureDoNotReturnContent(t *testing.T) {
	s, r, a := fixture()
	ctx := context.Background()
	r.value.Visibility = "private"
	if _, err := s.Get(ctx, "p", "editor"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
	a.unavailable = true
	if _, err := s.Get(ctx, "p", "owner"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestCrossOrganizationAndInactiveGrantsAreRejected(t *testing.T) {
	s, r, _ := fixture()
	for _, id := range []string{"foreign-team", "disabled-team"} {
		_, err := s.SetACL(context.Background(), ACLCommand{PromptID: "p", UserID: "owner", Visibility: "team", ExpectedVersion: 2, Grants: []domain.Grant{{TeamID: id, Permission: "viewer"}}})
		if !errors.Is(err, domain.ErrForbidden) || r.value.Version != 2 {
			t.Fatal(err)
		}
	}
}
func TestRollbackPreservesHistoryAndArchiveIsTerminal(t *testing.T) {
	s, r, _ := fixture()
	ctx := context.Background()
	v, err := s.Rollback(ctx, RollbackCommand{PromptID: "p", UserID: "editor", ExpectedVersion: 2, TargetVersion: 1})
	if err != nil || v.Version != 3 || v.Content != "original" || r.history[1].Content != "original" {
		t.Fatal(v, err)
	}
	if _, err := s.SelectedVersion(ctx, "p", "viewer", "other-org", 1); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.Archive(ctx, ArchiveCommand{PromptID: "p", OwnerUserID: "owner", ExpectedVersion: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, UpdateCommand{PromptID: "p", OwnerUserID: "owner", Name: "x", ExpectedVersion: 4}); !errors.Is(err, domain.ErrArchived) {
		t.Fatal(err)
	}
}
func TestNewPromptDefaultsToPrivateAndKeepsWhitespace(t *testing.T) {
	s, _, _ := fixture()
	v, err := s.Create(context.Background(), CreateCommand{OwnerUserID: "owner", OrganizationID: "org", Name: " Prompt ", Content: "  exact\n"})
	if err != nil || v.Visibility != "private" || v.Name != "Prompt" || v.Content != "  exact\n" {
		t.Fatal(v, err)
	}
}
