package prompt

import (
	"context"
	"testing"
	"time"

	domain "media_agent/media_control/biz/domain/prompt"
)

type fakeRepository struct{ value *domain.Prompt }

func (f *fakeRepository) Create(_ context.Context, value *domain.Prompt) error {
	copy := *value
	f.value = &copy
	return nil
}
func (f *fakeRepository) ListOwned(context.Context, string, string) ([]domain.Prompt, error) {
	if f.value == nil {
		return nil, nil
	}
	return []domain.Prompt{*f.value}, nil
}
func (f *fakeRepository) GetOwned(context.Context, string, string) (*domain.Prompt, error) {
	if f.value == nil {
		return nil, domain.ErrNotFound
	}
	copy := *f.value
	return &copy, nil
}
func (f *fakeRepository) Update(_ context.Context, value *domain.Prompt, _ int64) error {
	copy := *value
	f.value = &copy
	return nil
}
func (f *fakeRepository) Archive(_ context.Context, value *domain.Prompt, _ int64) error {
	copy := *value
	f.value = &copy
	return nil
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func TestUpdateRequiresExpectedVersionAndIncrements(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{value: &domain.Prompt{ID: "p-1", OwnerUserID: "u-1", Name: "old", Content: "old", Status: domain.StatusActive, Version: 2, CreatedAt: now, UpdatedAt: now}}
	svc := New(repo, directTx{}, nil)
	if _, err := svc.Update(context.Background(), UpdateCommand{PromptID: "p-1", OwnerUserID: "u-1", Name: "new", Content: "new", ExpectedVersion: 1}); err != domain.ErrConflict {
		t.Fatalf("stale update error = %v", err)
	}
	value, err := svc.Update(context.Background(), UpdateCommand{PromptID: "p-1", OwnerUserID: "u-1", Name: "new", Content: "new", ExpectedVersion: 2})
	if err != nil || value.Version != 3 || value.Name != "new" {
		t.Fatalf("updated prompt = %+v, err=%v", value, err)
	}
}

func TestArchiveIsTerminalAndIdempotencyIsRejected(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{value: &domain.Prompt{ID: "p-1", OwnerUserID: "u-1", Name: "prompt", Content: "content", Status: domain.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}}
	svc := New(repo, directTx{}, nil)
	value, err := svc.Archive(context.Background(), ArchiveCommand{PromptID: "p-1", OwnerUserID: "u-1", ExpectedVersion: 1})
	if err != nil || value.Status != domain.StatusArchived || value.Version != 2 {
		t.Fatalf("archived prompt = %+v, err=%v", value, err)
	}
	if _, err := svc.Update(context.Background(), UpdateCommand{PromptID: "p-1", OwnerUserID: "u-1", Name: "again", Content: "again", ExpectedVersion: 2}); err != domain.ErrArchived {
		t.Fatalf("update archived error = %v", err)
	}
}
