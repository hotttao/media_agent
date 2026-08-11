package workspace

import (
	"context"
	"testing"
	"time"

	domain "media_agent/media_control/biz/domain/workspace"
	"media_agent/media_control/biz/shared/secret"
)

type fakeRepository struct{ value domain.ConfiguredWorkspace }

func (f *fakeRepository) Create(context.Context, *domain.Workspace, *domain.AgentConfiguration) error {
	return nil
}
func (f *fakeRepository) ListOwned(context.Context, string) ([]domain.ConfiguredWorkspace, error) {
	return []domain.ConfiguredWorkspace{f.value}, nil
}
func (f *fakeRepository) GetOwned(context.Context, string, string) (*domain.ConfiguredWorkspace, error) {
	value := f.value
	return &value, nil
}
func (f *fakeRepository) Update(_ context.Context, workspace *domain.Workspace, configuration *domain.AgentConfiguration) error {
	f.value = domain.ConfiguredWorkspace{Workspace: *workspace, AgentConfiguration: *configuration}
	return nil
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func TestUpdateKeepsSecretWhenAPIKeyIsBlank(t *testing.T) {
	cipher := secret.New("test-key")
	encrypted, err := cipher.Encrypt("existing-secret")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	repo := &fakeRepository{value: domain.ConfiguredWorkspace{
		Workspace:          domain.Workspace{ID: "workspace-1", OwnerUserID: "owner-1", Name: "Before", Version: 1, CreatedAt: now, UpdatedAt: now},
		AgentConfiguration: domain.AgentConfiguration{ID: "config-1", WorkspaceID: "workspace-1", AgentID: "codex", AgentType: "codex", EncryptedAPIKey: encrypted, Version: 1, CreatedAt: now, UpdatedAt: now},
	}}
	service := New(repo, directTx{}, cipher)

	value, err := service.Update(context.Background(), UpdateCommand{WorkspaceID: "workspace-1", OwnerUserID: "owner-1", Name: " After ", AgentType: "CODEX"})
	if err != nil {
		t.Fatal(err)
	}
	if value.Name != "After" {
		t.Fatalf("name = %q", value.Name)
	}
	if value.AgentConfiguration.Version != 2 {
		t.Fatalf("configuration version = %d", value.AgentConfiguration.Version)
	}
	plain, err := cipher.Decrypt(value.EncryptedAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "existing-secret" {
		t.Fatalf("secret changed to %q", plain)
	}
}
