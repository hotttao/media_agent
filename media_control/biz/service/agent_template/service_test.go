package agent_template

import (
	"context"
	"testing"
	"time"

	domain "media_agent/media_control/biz/domain/agent_template"
)

type fakeRepository struct{ value *domain.AgentTemplate }

func (f *fakeRepository) Create(_ context.Context, value *domain.AgentTemplate) error {
	copy := *value
	f.value = &copy
	return nil
}
func (f *fakeRepository) ListOwned(context.Context, string, string) ([]domain.AgentTemplate, error) {
	if f.value == nil {
		return nil, nil
	}
	return []domain.AgentTemplate{*f.value}, nil
}
func (f *fakeRepository) GetOwned(context.Context, string, string) (*domain.AgentTemplate, error) {
	if f.value == nil {
		return nil, domain.ErrNotFound
	}
	copy := *f.value
	return &copy, nil
}
func (f *fakeRepository) Update(_ context.Context, value *domain.AgentTemplate, _ int64) error {
	copy := *value
	f.value = &copy
	return nil
}
func (f *fakeRepository) Archive(_ context.Context, value *domain.AgentTemplate, _ int64) error {
	copy := *value
	f.value = &copy
	return nil
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func TestTemplateRejectsInvalidJSON(t *testing.T) {
	svc := New(&fakeRepository{}, directTx{}, nil)
	if _, err := svc.Create(context.Background(), CreateCommand{OwnerUserID: "u-1", Name: "bad", ConfigurationJSON: "{"}); err == nil {
		t.Fatal("invalid JSON was accepted")
	}
}

func TestTemplateUpdateUsesOptimisticVersion(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{value: &domain.AgentTemplate{ID: "t-1", OwnerUserID: "u-1", Name: "old", ConfigurationJSON: `{"model":"x"}`, Status: domain.StatusActive, Version: 2, CreatedAt: now, UpdatedAt: now}}
	svc := New(repo, directTx{}, nil)
	if _, err := svc.Update(context.Background(), UpdateCommand{TemplateID: "t-1", OwnerUserID: "u-1", Name: "new", ConfigurationJSON: `{}`, ExpectedVersion: 1}); err != domain.ErrConflict {
		t.Fatalf("stale update error = %v", err)
	}
	value, err := svc.Update(context.Background(), UpdateCommand{TemplateID: "t-1", OwnerUserID: "u-1", Name: "new", ConfigurationJSON: `{}`, ExpectedVersion: 2})
	if err != nil || value.Version != 3 {
		t.Fatalf("updated template = %+v, err=%v", value, err)
	}
}
