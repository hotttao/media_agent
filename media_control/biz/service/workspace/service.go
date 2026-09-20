package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"media_agent/media_control/biz/dal"
	domain "media_agent/media_control/biz/domain/workspace"
	"media_agent/media_control/biz/shared/secret"
)

type CreateCommand struct{ UserID, OrganizationID, TeamID, Name, AgentType, APIKey string }
type UpdateCommand struct{ WorkspaceID, UserID, Name, AgentType, APIKey string }

var ErrForbidden = errors.New("workspace access denied")
var ErrInvalidAction = errors.New("invalid workspace action")

type Service struct {
	repo   domain.AccessRepository
	tx     dal.TxRunner
	cipher *secret.Cipher
	teams  domain.TeamAccess
}

func New(repo domain.AccessRepository, tx dal.TxRunner, cipher *secret.Cipher, teams domain.TeamAccess) *Service {
	return &Service{repo: repo, tx: tx, cipher: cipher, teams: teams}
}

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.ConfiguredWorkspace, error) {
	cmd.Name = strings.TrimSpace(cmd.Name)
	if cmd.UserID == "" || cmd.OrganizationID == "" || cmd.TeamID == "" || cmd.Name == "" {
		return nil, fmt.Errorf("user, organization_id, team_id and name are required")
	}
	decision, err := s.teams.Authorize(ctx, cmd.UserID, cmd.TeamID, "create_workspace")
	if err != nil {
		return nil, fmt.Errorf("authorize team: %w", err)
	}
	if !decision.Allowed || decision.OrganizationID != cmd.OrganizationID {
		return nil, ErrForbidden
	}
	agentType, err := domain.ParseAgentType(cmd.AgentType)
	if err != nil {
		return nil, err
	}
	encrypted, err := s.cipher.Encrypt(strings.TrimSpace(cmd.APIKey))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	workspaceID := uuid.NewString()
	w := &domain.Workspace{ID: workspaceID, OrganizationID: cmd.OrganizationID, TeamID: cmd.TeamID, CreatedBy: cmd.UserID, OwnerUserID: cmd.UserID, Name: cmd.Name, DesiredState: "running", Version: 1, CreatedAt: now, UpdatedAt: now}
	a := &domain.AgentConfiguration{ID: uuid.NewString(), WorkspaceID: workspaceID, AgentID: string(agentType), AgentType: string(agentType), EncryptedAPIKey: encrypted, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.tx.RunInTx(ctx, func(txCtx context.Context) error { return s.repo.Create(txCtx, w, a) }); err != nil {
		return nil, err
	}
	return &domain.ConfiguredWorkspace{Workspace: *w, AgentConfiguration: *a}, nil
}

func (s *Service) List(ctx context.Context, userID string) ([]domain.ConfiguredWorkspace, error) {
	teamIDs, err := s.teams.ListTeams(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list authorized teams: %w", err)
	}
	return s.repo.ListByTeams(ctx, teamIDs)
}
func (s *Service) Get(ctx context.Context, id, userID string) (*domain.ConfiguredWorkspace, error) {
	value, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if value.TeamID == "" {
		return nil, domain.ErrNotFound
	}
	decision, err := s.teams.Authorize(ctx, userID, value.TeamID, "view")
	if err != nil {
		return nil, fmt.Errorf("authorize workspace view: %w", err)
	}
	if !decision.Allowed || decision.OrganizationID != value.OrganizationID {
		return nil, domain.ErrNotFound
	}
	return value, nil
}
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.ConfiguredWorkspace, error) {
	cmd.Name = strings.TrimSpace(cmd.Name)
	if cmd.WorkspaceID == "" || cmd.UserID == "" || cmd.Name == "" {
		return nil, fmt.Errorf("workspace, user and name are required")
	}
	agentType, err := domain.ParseAgentType(cmd.AgentType)
	if err != nil {
		return nil, err
	}
	value, err := s.repo.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if value.TeamID == "" {
		return nil, domain.ErrNotFound
	}
	decision, err := s.teams.Authorize(ctx, cmd.UserID, value.TeamID, "manage")
	if err != nil {
		return nil, fmt.Errorf("authorize workspace manage: %w", err)
	}
	if !decision.Allowed || decision.OrganizationID != value.OrganizationID {
		return nil, ErrForbidden
	}
	secretValue := value.EncryptedAPIKey
	configurationChanged := value.AgentType != string(agentType)
	newAPIKey := strings.TrimSpace(cmd.APIKey)
	if newAPIKey != "" {
		oldAPIKey, decryptErr := s.cipher.Decrypt(value.EncryptedAPIKey)
		if decryptErr != nil {
			return nil, decryptErr
		}
		if oldAPIKey != newAPIKey {
			configurationChanged = true
			secretValue, err = s.cipher.Encrypt(newAPIKey)
			if err != nil {
				return nil, err
			}
		}
	}
	now := time.Now().UTC()
	if value.Name != cmd.Name {
		value.Workspace.Version++
	}
	value.Name = cmd.Name
	value.Workspace.UpdatedAt = now
	value.AgentID = string(agentType)
	value.AgentType = string(agentType)
	value.EncryptedAPIKey = secretValue
	if configurationChanged {
		value.AgentConfiguration.Version++
		value.AgentConfiguration.UpdatedAt = now
	}
	if err := s.tx.RunInTx(ctx, func(txCtx context.Context) error {
		return s.repo.Update(txCtx, &value.Workspace, &value.AgentConfiguration)
	}); err != nil {
		return nil, err
	}
	return value, nil
}
func (s *Service) RuntimeContext(ctx context.Context, id, userID string) (*domain.ConfiguredWorkspace, string, error) {
	value, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if value.TeamID == "" {
		return nil, "", domain.ErrNotFound
	}
	decision, err := s.teams.Authorize(ctx, userID, value.TeamID, "manage")
	if err != nil {
		return nil, "", fmt.Errorf("authorize runtime context: %w", err)
	}
	if !decision.Allowed || decision.OrganizationID != value.OrganizationID {
		return nil, "", ErrForbidden
	}
	apiKey, err := s.cipher.Decrypt(value.EncryptedAPIKey)
	return value, apiKey, err
}

func (s *Service) CheckAccess(ctx context.Context, workspaceID, userID, action string) (bool, error) {
	value, err := s.repo.Get(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	if value.TeamID == "" || value.OrganizationID == "" || value.DesiredState == "deleted" {
		return false, nil
	}
	permission := action
	switch action {
	case "view", "manage", "use_materials", "use_workflow":
	default:
		return false, ErrInvalidAction
	}
	decision, err := s.teams.Authorize(ctx, userID, value.TeamID, permission)
	if err != nil {
		return false, fmt.Errorf("authorize workspace %s: %w", action, err)
	}
	return decision.Allowed && decision.OrganizationID == value.OrganizationID, nil
}
