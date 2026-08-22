package provider

import (
	"context"
	domain "media_agent/media_control/biz/domain/provider"
	"testing"
	"time"
)

type fakeRepository struct{ value *domain.Provider }

func (f *fakeRepository) Create(_ context.Context, v *domain.Provider) error {
	c := *v
	f.value = &c
	return nil
}
func (f *fakeRepository) List(context.Context) ([]domain.Provider, error) {
	if f.value == nil {
		return nil, nil
	}
	return []domain.Provider{*f.value}, nil
}
func (f *fakeRepository) Get(context.Context, string) (*domain.Provider, error) {
	if f.value == nil {
		return nil, domain.ErrNotFound
	}
	c := *f.value
	return &c, nil
}
func (f *fakeRepository) Update(_ context.Context, v *domain.Provider, _ int64) error {
	c := *v
	f.value = &c
	return nil
}
func (f *fakeRepository) SetStatus(_ context.Context, v *domain.Provider, _ int64) error {
	c := *v
	f.value = &c
	return nil
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }
func TestProviderRejectsInvalidCapabilities(t *testing.T) {
	svc := New(&fakeRepository{}, directTx{})
	if _, err := svc.Create(context.Background(), CreateCommand{Name: "x", ProviderType: "model", Endpoint: "https://x", CapabilitiesJSON: "{"}); err == nil {
		t.Fatal("invalid capabilities accepted")
	}
}
func TestProviderStatusVersion(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{value: &domain.Provider{ID: "p-1", Name: "x", ProviderType: "model", Endpoint: "https://x", CapabilitiesJSON: `{}`, Status: domain.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}}
	svc := New(repo, directTx{})
	v, err := svc.SetStatus(context.Background(), StatusCommand{ProviderID: "p-1", Status: "disabled", ExpectedVersion: 1})
	if err != nil || v.Status != domain.StatusDisabled || v.Version != 2 {
		t.Fatalf("provider=%+v err=%v", v, err)
	}
	if _, err := svc.SetStatus(context.Background(), StatusCommand{ProviderID: "p-1", Status: "active", ExpectedVersion: 1}); err != domain.ErrConflict {
		t.Fatalf("stale status err=%v", err)
	}
}
