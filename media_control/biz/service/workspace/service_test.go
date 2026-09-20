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
func (f *fakeRepository) ListByTeams(context.Context, []string) ([]domain.ConfiguredWorkspace, error) {
	return []domain.ConfiguredWorkspace{f.value}, nil
}
func (f *fakeRepository) Get(context.Context, string) (*domain.ConfiguredWorkspace, error) {
	value := f.value
	return &value, nil
}
func (f *fakeRepository) Update(_ context.Context, workspace *domain.Workspace, configuration *domain.AgentConfiguration) error {
	f.value = domain.ConfiguredWorkspace{Workspace: *workspace, AgentConfiguration: *configuration}
	return nil
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type allowTeams struct{}

func (allowTeams) ListTeams(context.Context, string) ([]string, error) {
	return []string{"team-1"}, nil
}

type denyTeams struct{ allowTeams }

func (denyTeams) Authorize(_ context.Context, _ string, teamID, _ string) (domain.TeamAuthorization, error) {
	return domain.TeamAuthorization{Allowed: false, OrganizationID: "org-1", TeamID: teamID}, nil
}
func (allowTeams) Authorize(_ context.Context, _ string, teamID, _ string) (domain.TeamAuthorization, error) {
	return domain.TeamAuthorization{Allowed: true, OrganizationID: "org-1", TeamID: teamID}, nil
}

func TestUpdateKeepsSecretWhenAPIKeyIsBlank(t *testing.T) {
	cipher := secret.New("test-key")
	encrypted, err := cipher.Encrypt("existing-secret")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	repo := &fakeRepository{value: domain.ConfiguredWorkspace{
		Workspace:          domain.Workspace{ID: "workspace-1", OrganizationID: "org-1", TeamID: "team-1", CreatedBy: "owner-1", OwnerUserID: "owner-1", Name: "Before", Version: 1, CreatedAt: now, UpdatedAt: now},
		AgentConfiguration: domain.AgentConfiguration{ID: "config-1", WorkspaceID: "workspace-1", AgentID: "codex", AgentType: "codex", EncryptedAPIKey: encrypted, Version: 1, CreatedAt: now, UpdatedAt: now},
	}}
	service := New(repo, directTx{}, cipher, allowTeams{})

	value, err := service.Update(context.Background(), UpdateCommand{WorkspaceID: "workspace-1", UserID: "owner-1", Name: " After ", AgentType: "CODEX"})
	if err != nil {
		t.Fatal(err)
	}
	if value.Name != "After" {
		t.Fatalf("name = %q", value.Name)
	}
	if value.AgentConfiguration.Version != 1 {
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

func TestCreateAcceptsClaudeCodeAndEncryptsAPIKey(t *testing.T) {
	cipher := secret.New("test-key")
	service := New(&fakeRepository{}, directTx{}, cipher, allowTeams{})

	value, err := service.Create(context.Background(), CreateCommand{
		UserID: "owner-1", OrganizationID: "org-1", TeamID: "team-1", Name: "Claude", AgentType: "CLAUDE-CODE", APIKey: "sk-ant-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if value.AgentType != "claude-code" || value.AgentID != "claude-code" {
		t.Fatalf("unexpected Agent Configuration: %+v", value.AgentConfiguration)
	}
	plain, err := cipher.Decrypt(value.EncryptedAPIKey)
	if err != nil || plain != "sk-ant-test" {
		t.Fatalf("stored API Key = %q, err=%v", plain, err)
	}
}

func TestUpdateOnlyAdvancesConfigurationVersionWhenConfigurationChanges(t *testing.T) {
	cipher := secret.New("test-key")
	encrypted, err := cipher.Encrypt("same-secret")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	repo := &fakeRepository{value: domain.ConfiguredWorkspace{
		Workspace:          domain.Workspace{ID: "workspace-1", OrganizationID: "org-1", TeamID: "team-1", CreatedBy: "owner-1", OwnerUserID: "owner-1", Name: "Before", Version: 1, CreatedAt: now, UpdatedAt: now},
		AgentConfiguration: domain.AgentConfiguration{ID: "config-1", WorkspaceID: "workspace-1", AgentID: "codex", AgentType: "codex", EncryptedAPIKey: encrypted, Version: 4, CreatedAt: now, UpdatedAt: now},
	}}
	service := New(repo, directTx{}, cipher, allowTeams{})

	unchanged, err := service.Update(context.Background(), UpdateCommand{WorkspaceID: "workspace-1", UserID: "owner-1", Name: "Renamed", AgentType: "codex", APIKey: "same-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.AgentConfiguration.Version != 4 || unchanged.Workspace.Version != 2 {
		t.Fatalf("unexpected versions after name-only update: workspace=%d config=%d", unchanged.Workspace.Version, unchanged.AgentConfiguration.Version)
	}

	changed, err := service.Update(context.Background(), UpdateCommand{WorkspaceID: "workspace-1", UserID: "owner-1", Name: "Renamed", AgentType: "claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	if changed.AgentConfiguration.Version != 5 || changed.AgentType != "claude-code" {
		t.Fatalf("unexpected changed configuration: %+v", changed.AgentConfiguration)
	}
}

func TestCreateRequiresTeamAuthorizationAndMatchingOrganization(t *testing.T) {
	cipher := secret.New("test-key")
	command := CreateCommand{UserID: "user-1", OrganizationID: "org-1", TeamID: "team-1", Name: "Denied", AgentType: "codex"}
	if _, err := New(&fakeRepository{}, directTx{}, cipher, denyTeams{}).Create(context.Background(), command); err != ErrForbidden {
		t.Fatalf("denied create error=%v, want ErrForbidden", err)
	}

	mismatch := teamAccessFunc(func(context.Context, string, string, string) (domain.TeamAuthorization, error) {
		return domain.TeamAuthorization{Allowed: true, OrganizationID: "other-org", TeamID: "team-1"}, nil
	})
	if _, err := New(&fakeRepository{}, directTx{}, cipher, mismatch).Create(context.Background(), command); err != ErrForbidden {
		t.Fatalf("cross-organization create error=%v, want ErrForbidden", err)
	}
}

func TestCheckAccessUsesPersistedWorkspaceTeam(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{value: domain.ConfiguredWorkspace{Workspace: domain.Workspace{
		ID: "workspace-1", OrganizationID: "org-1", TeamID: "team-1", CreatedBy: "creator", Name: "Workspace", DesiredState: "running", CreatedAt: now, UpdatedAt: now,
	}}}
	var checkedTeam, checkedPermission string
	teams := teamAccessFunc(func(_ context.Context, _ string, teamID, permission string) (domain.TeamAuthorization, error) {
		checkedTeam, checkedPermission = teamID, permission
		return domain.TeamAuthorization{Allowed: true, OrganizationID: "org-1", TeamID: teamID}, nil
	})
	allowed, err := New(repo, directTx{}, secret.New("test-key"), teams).CheckAccess(context.Background(), "workspace-1", "user-1", "use_materials")
	if err != nil || !allowed {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
	if checkedTeam != "team-1" || checkedPermission != "use_materials" {
		t.Fatalf("checked team=%q permission=%q", checkedTeam, checkedPermission)
	}
}

type teamAccessFunc func(context.Context, string, string, string) (domain.TeamAuthorization, error)

func (f teamAccessFunc) ListTeams(context.Context, string) ([]string, error) {
	return []string{"team-1"}, nil
}
func (f teamAccessFunc) Authorize(ctx context.Context, userID, teamID, permission string) (domain.TeamAuthorization, error) {
	return f(ctx, userID, teamID, permission)
}
