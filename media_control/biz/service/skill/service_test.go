package skill

import (
	"context"
	"testing"
	"time"

	domain "media_agent/media_control/biz/domain/skill"
)

type fakeRepository struct{ value *domain.Skill }

func (f *fakeRepository) Create(_ context.Context, value *domain.Skill) error {
	copy := *value
	f.value = &copy
	return nil
}
func (f *fakeRepository) ListOwned(context.Context, string, string) ([]domain.Skill, error) {
	if f.value == nil {
		return nil, nil
	}
	return []domain.Skill{*f.value}, nil
}
func (f *fakeRepository) GetOwned(context.Context, string, string) (*domain.Skill, error) {
	if f.value == nil {
		return nil, domain.ErrNotFound
	}
	copy := *f.value
	return &copy, nil
}
func (f *fakeRepository) Update(_ context.Context, value *domain.Skill, _ int64) error {
	copy := *value
	f.value = &copy
	return nil
}
func (f *fakeRepository) SetStatus(_ context.Context, value *domain.Skill, _ int64) error {
	copy := *value
	f.value = &copy
	return nil
}
func (f *fakeRepository) Archive(_ context.Context, value *domain.Skill, _ int64) error {
	copy := *value
	f.value = &copy
	return nil
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func TestCreateRejectsUnsafeContentPath(t *testing.T) {
	svc := New(&fakeRepository{}, directTx{}, nil)
	if _, err := svc.Create(context.Background(), CreateCommand{OwnerUserID: "u-1", Name: "unsafe", ContentPath: "../secret", Checksum: "sha256:x"}); err == nil {
		t.Fatal("unsafe path was accepted")
	}
}

func TestSetStatusAndArchiveUseVersion(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{value: &domain.Skill{ID: "s-1", OwnerUserID: "u-1", Name: "skill", ContentPath: "skills/a.md", Checksum: "sha256:a", Status: domain.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}}
	svc := New(repo, directTx{}, nil)
	value, err := svc.SetStatus(context.Background(), StatusCommand{SkillID: "s-1", OwnerUserID: "u-1", Status: "disabled", ExpectedVersion: 1})
	if err != nil || value.Status != domain.StatusDisabled || value.Version != 2 {
		t.Fatalf("disabled skill = %+v, err=%v", value, err)
	}
	if _, err := svc.Archive(context.Background(), ArchiveCommand{SkillID: "s-1", OwnerUserID: "u-1", ExpectedVersion: 1}); err != domain.ErrConflict {
		t.Fatalf("stale archive error = %v", err)
	}
	value, err = svc.Archive(context.Background(), ArchiveCommand{SkillID: "s-1", OwnerUserID: "u-1", ExpectedVersion: 2})
	if err != nil || value.Status != domain.StatusArchived {
		t.Fatalf("archived skill = %+v, err=%v", value, err)
	}
}
