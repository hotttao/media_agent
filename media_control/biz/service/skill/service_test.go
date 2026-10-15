package skill

import (
	"context"
	"errors"
	"media_agent/hertz_infra/authn/resourceaccess"
	d "media_agent/media_control/biz/domain/skill"
	"strings"
	"testing"
	"time"
)

type fakeRepository struct {
	value   *d.Skill
	history map[int64]d.Version
}

func (r *fakeRepository) Get(context.Context, string) (*d.Skill, error) {
	if r.value == nil {
		return nil, d.ErrNotFound
	}
	v := *r.value
	return &v, nil
}
func (r *fakeRepository) List(context.Context, string, string, []string) ([]d.Skill, error) {
	return []d.Skill{*r.value}, nil
}
func (r *fakeRepository) Save(_ context.Context, v *d.Skill, expected int64) error {
	if expected != 0 && r.value.Version != expected {
		return d.ErrConflict
	}
	copy := *v
	r.value = &copy
	return nil
}
func (r *fakeRepository) CreateVersion(_ context.Context, v *d.Version) error {
	if r.history == nil {
		r.history = map[int64]d.Version{}
	}
	if _, ok := r.history[v.Version]; ok {
		return d.ErrConflict
	}
	r.history[v.Version] = *v
	return nil
}
func (r *fakeRepository) Versions(context.Context, string) ([]d.Version, error) {
	result := []d.Version{}
	for _, v := range r.history {
		result = append(result, v)
	}
	return result, nil
}
func (r *fakeRepository) GetVersion(_ context.Context, _ string, version int64) (*d.Version, error) {
	v, ok := r.history[version]
	if !ok {
		return nil, d.ErrNotFound
	}
	return &v, nil
}
func (r *fakeRepository) ListUnassigned(context.Context, string) ([]d.Skill, error) { return nil, nil }
func (r *fakeRepository) AssignOrganization(ctx context.Context, v *d.Skill, expected int64) error {
	return r.Save(ctx, v, expected)
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type access struct{}

func (access) Resolve(_ context.Context, user, org string) (resourceaccess.Facts, error) {
	return resourceaccess.Facts{Member: org == "org" && user != "outsider", TeamIDs: []string{user}}, nil
}
func (access) Team(_ context.Context, _ string, id string) (resourceaccess.Team, error) {
	return resourceaccess.Team{TeamID: id, OrganizationID: "org", TeamActive: true, OrganizationActive: true}, nil
}
func fixture() (*Service, *fakeRepository) {
	now := time.Now().UTC()
	r := &fakeRepository{value: &d.Skill{ID: "skill", OrganizationID: "org", OwnerUserID: "owner", Name: "Example", Status: d.StatusActive, Visibility: "team", Grants: []d.Grant{{TeamID: "editor", Permission: "editor"}, {TeamID: "viewer", Permission: "viewer"}}, Version: 3, PublishedVersion: 1, Checksum: strings.Repeat("a", 64), DraftChecksum: strings.Repeat("b", 64), CreatedAt: now, UpdatedAt: now}, history: map[int64]d.Version{1: {SkillID: "skill", Version: 1, Checksum: strings.Repeat("a", 64), Slug: "example"}}}
	return New(r, directTx{}, access{}), r
}
func TestPublishedContentRemainsIndependentOfDraftEdits(t *testing.T) {
	s, r := fixture()
	ctx := context.Background()
	v, err := s.Publish(ctx, "skill", "editor", 3, r.value.DraftChecksum, r.value.DraftChecksum, "example", "Example")
	if err != nil || v.PublishedVersion != 2 || v.Version != 4 {
		t.Fatal(v, err)
	}
	_, err = s.SaveDraft(ctx, "skill", "editor", 4, strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	selected, err := s.SelectedVersion(ctx, "skill", "viewer", "org", 2)
	if err != nil || selected.Checksum != strings.Repeat("b", 64) || r.history[1].Checksum != strings.Repeat("a", 64) {
		t.Fatal(selected, err)
	}
	if _, err := s.SelectedVersion(ctx, "skill", "viewer", "foreign", 2); !errors.Is(err, d.ErrForbidden) {
		t.Fatal(err)
	}
}
func TestViewerCannotEditAndEditorCannotChangeSourcesACLOrStatus(t *testing.T) {
	s, _ := fixture()
	ctx := context.Background()
	if _, err := s.SaveDraft(ctx, "skill", "viewer", 3, strings.Repeat("c", 64)); !errors.Is(err, d.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.SaveSource(ctx, "skill", "editor", 3, d.GitSource{URL: "https://git.example.invalid/repo.git"}); !errors.Is(err, d.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.SetACL(ctx, ACLCommand{SkillID: "skill", UserID: "editor", Visibility: "public", ExpectedVersion: 3}); !errors.Is(err, d.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.SetStatus(ctx, StatusCommand{SkillID: "skill", OwnerUserID: "editor", Status: "disabled", ExpectedVersion: 3}); !errors.Is(err, d.ErrForbidden) {
		t.Fatal(err)
	}
}
func TestDisabledSkillCannotBeNewlySelectedAndArchiveIsTerminal(t *testing.T) {
	s, r := fixture()
	ctx := context.Background()
	if _, err := s.SetStatus(ctx, StatusCommand{SkillID: "skill", OwnerUserID: "owner", Status: "disabled", ExpectedVersion: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SelectedVersion(ctx, "skill", "viewer", "org", 1); !errors.Is(err, d.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := s.Version(ctx, "skill", "viewer", 1); err != nil {
		t.Fatal("history lost", err)
	}
	if _, err := s.Archive(ctx, ArchiveCommand{SkillID: "skill", OwnerUserID: "owner", ExpectedVersion: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveDraft(ctx, "skill", "owner", r.value.Version, strings.Repeat("d", 64)); !errors.Is(err, d.ErrArchived) {
		t.Fatal(err)
	}
}
