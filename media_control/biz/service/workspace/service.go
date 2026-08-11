package workspace

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"media_agent/media_control/biz/dal"
	domain "media_agent/media_control/biz/domain/workspace"
	"media_agent/media_control/biz/shared/secret"
)

type CreateCommand struct{ OwnerUserID, Name, AgentType, APIKey string }
type UpdateCommand struct{ WorkspaceID, OwnerUserID, Name, AgentType, APIKey string }

type Service struct {
	repo   domain.Repository
	tx     dal.TxRunner
	cipher *secret.Cipher
}

func New(repo domain.Repository, tx dal.TxRunner, cipher *secret.Cipher) *Service {
	return &Service{repo: repo, tx: tx, cipher: cipher}
}

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.ConfiguredWorkspace, error) {
	cmd.Name = strings.TrimSpace(cmd.Name)
	cmd.AgentType = strings.ToLower(strings.TrimSpace(cmd.AgentType))
	if cmd.OwnerUserID == "" || cmd.Name == "" {
		return nil, fmt.Errorf("owner and name are required")
	}
	if cmd.AgentType != "codex" {
		return nil, fmt.Errorf("unsupported agent type %q", cmd.AgentType)
	}
	encrypted, err := s.cipher.Encrypt(strings.TrimSpace(cmd.APIKey))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	workspaceID := uuid.NewString()
	w := &domain.Workspace{ID: workspaceID, OwnerUserID: cmd.OwnerUserID, Name: cmd.Name, DesiredState: "running", Version: 1, CreatedAt: now, UpdatedAt: now}
	a := &domain.AgentConfiguration{ID: uuid.NewString(), WorkspaceID: workspaceID, AgentID: cmd.AgentType, AgentType: cmd.AgentType, EncryptedAPIKey: encrypted, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.tx.RunInTx(ctx, func(txCtx context.Context) error { return s.repo.Create(txCtx, w, a) }); err != nil {
		return nil, err
	}
	return &domain.ConfiguredWorkspace{Workspace: *w, AgentConfiguration: *a}, nil
}

func (s *Service) List(ctx context.Context, owner string) ([]domain.ConfiguredWorkspace, error) {
	return s.repo.ListOwned(ctx, owner)
}
func (s *Service) Get(ctx context.Context, id, owner string) (*domain.ConfiguredWorkspace, error) {
	return s.repo.GetOwned(ctx, id, owner)
}
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.ConfiguredWorkspace, error) {
	cmd.Name = strings.TrimSpace(cmd.Name)
	cmd.AgentType = strings.ToLower(strings.TrimSpace(cmd.AgentType))
	if cmd.WorkspaceID == "" || cmd.OwnerUserID == "" || cmd.Name == "" {
		return nil, fmt.Errorf("workspace, owner and name are required")
	}
	if cmd.AgentType != "codex" {
		return nil, fmt.Errorf("unsupported agent type %q", cmd.AgentType)
	}
	value, err := s.repo.GetOwned(ctx, cmd.WorkspaceID, cmd.OwnerUserID)
	if err != nil {
		return nil, err
	}
	secretValue := value.EncryptedAPIKey
	if strings.TrimSpace(cmd.APIKey) != "" {
		secretValue, err = s.cipher.Encrypt(strings.TrimSpace(cmd.APIKey))
		if err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	value.Name = cmd.Name
	value.Workspace.Version++
	value.Workspace.UpdatedAt = now
	value.AgentID = cmd.AgentType
	value.AgentType = cmd.AgentType
	value.EncryptedAPIKey = secretValue
	value.AgentConfiguration.Version++
	value.AgentConfiguration.UpdatedAt = now
	if err := s.tx.RunInTx(ctx, func(txCtx context.Context) error {
		return s.repo.Update(txCtx, &value.Workspace, &value.AgentConfiguration)
	}); err != nil {
		return nil, err
	}
	return value, nil
}
func (s *Service) RuntimeContext(ctx context.Context, id, owner string) (*domain.ConfiguredWorkspace, string, error) {
	value, err := s.repo.GetOwned(ctx, id, owner)
	if err != nil {
		return nil, "", err
	}
	apiKey, err := s.cipher.Decrypt(value.EncryptedAPIKey)
	return value, apiKey, err
}
