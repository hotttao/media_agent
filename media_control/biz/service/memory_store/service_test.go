package memory_store

import (
	"context"
	domain "media_agent/media_control/biz/domain/memory_store"
	"testing"
	"time"
)

type fakeRepository struct{ value *domain.MemoryStore }

func (f *fakeRepository) Create(_ context.Context, v *domain.MemoryStore) error {
	c := *v
	f.value = &c
	return nil
}
func (f *fakeRepository) ListOwned(context.Context, string, string) ([]domain.MemoryStore, error) {
	if f.value == nil {
		return nil, nil
	}
	return []domain.MemoryStore{*f.value}, nil
}
func (f *fakeRepository) GetOwned(context.Context, string, string) (*domain.MemoryStore, error) {
	if f.value == nil {
		return nil, domain.ErrNotFound
	}
	c := *f.value
	return &c, nil
}
func (f *fakeRepository) Update(_ context.Context, v *domain.MemoryStore, _ int64) error {
	c := *v
	f.value = &c
	return nil
}
func (f *fakeRepository) SetStatus(_ context.Context, v *domain.MemoryStore, _ int64) error {
	c := *v
	f.value = &c
	return nil
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }
func TestCreateRejectsInvalidJSON(t *testing.T) {
	svc := New(&fakeRepository{}, directTx{}, nil, nil)
	if _, err := svc.Create(context.Background(), CreateCommand{OwnerUserID: "u", WorkspaceID: "w", Name: "m", StoreType: "redis", ConfigurationJSON: "{"}); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}
func TestStatusUsesVersion(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{value: &domain.MemoryStore{ID: "m-1", WorkspaceID: "w", OwnerUserID: "u", Name: "m", StoreType: "redis", ConfigurationJSON: `{}`, Status: domain.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}}
	svc := New(repo, directTx{}, nil, nil)
	v, err := svc.SetStatus(context.Background(), StatusCommand{StoreID: "m-1", OwnerUserID: "u", Status: "disabled", ExpectedVersion: 1})
	if err != nil || v.Status != domain.StatusDisabled || v.Version != 2 {
		t.Fatalf("store=%+v err=%v", v, err)
	}
	if _, err := svc.SetStatus(context.Background(), StatusCommand{StoreID: "m-1", OwnerUserID: "u", Status: "active", ExpectedVersion: 1}); err != domain.ErrConflict {
		t.Fatalf("stale status err=%v", err)
	}
}
