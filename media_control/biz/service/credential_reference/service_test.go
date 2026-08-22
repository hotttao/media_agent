package credential_reference

import (
	"context"
	domain "media_agent/media_control/biz/domain/credential_reference"
	"testing"
	"time"
)

type fakeRepository struct{ value *domain.CredentialReference }

func (f *fakeRepository) Create(_ context.Context, v *domain.CredentialReference, _ string) error {
	c := *v
	f.value = &c
	return nil
}
func (f *fakeRepository) ListOwned(context.Context, string, string) ([]domain.CredentialReference, error) {
	if f.value == nil {
		return nil, nil
	}
	return []domain.CredentialReference{*f.value}, nil
}
func (f *fakeRepository) GetOwned(context.Context, string, string) (*domain.CredentialReference, error) {
	if f.value == nil {
		return nil, domain.ErrNotFound
	}
	c := *f.value
	return &c, nil
}
func (f *fakeRepository) Update(_ context.Context, v *domain.CredentialReference, _ string, _ int64) error {
	c := *v
	f.value = &c
	return nil
}
func (f *fakeRepository) Revoke(_ context.Context, v *domain.CredentialReference, _ int64) error {
	c := *v
	f.value = &c
	return nil
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }
func TestCreateRejectsSecretTextWithNewline(t *testing.T) {
	svc := New(&fakeRepository{}, directTx{}, nil, nil)
	if _, err := svc.Create(context.Background(), CreateCommand{OwnerUserID: "u", Name: "x", ProviderID: "p", SecretRef: "vault://x\nsecret"}); err == nil {
		t.Fatal("secret text accepted")
	}
}
func TestRevokeUsesVersionAndNeverReturnsSecretRef(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{value: &domain.CredentialReference{ID: "c-1", OwnerUserID: "u", Name: "ref", ProviderID: "p", Status: domain.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}}
	svc := New(repo, directTx{}, nil, nil)
	v, err := svc.Revoke(context.Background(), RevokeCommand{CredentialID: "c-1", OwnerUserID: "u", ExpectedVersion: 1})
	if err != nil || v.Status != domain.StatusRevoked || v.Version != 2 {
		t.Fatalf("ref=%+v err=%v", v, err)
	}
}
