package agent_template

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"media_agent/media_control/biz/dal"
	domain "media_agent/media_control/biz/domain/agent_template"
	workspace "media_agent/media_control/biz/domain/workspace"
)

type CreateCommand struct{ OwnerUserID, WorkspaceID, Name, ConfigurationJSON string }
type UpdateCommand struct {
	TemplateID, OwnerUserID, Name, ConfigurationJSON string
	ExpectedVersion                                  int64
}
type ArchiveCommand struct {
	TemplateID, OwnerUserID string
	ExpectedVersion         int64
}

type Service struct {
	repo          domain.Repository
	workspaceRepo workspace.Repository
	tx            dal.TxRunner
}

func New(repo domain.Repository, tx dal.TxRunner, workspaceRepo workspace.Repository) *Service {
	return &Service{repo: repo, tx: tx, workspaceRepo: workspaceRepo}
}

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.AgentTemplate, error) {
	if strings.TrimSpace(cmd.OwnerUserID) == "" {
		return nil, fmt.Errorf("owner is required")
	}
	name, configuration, err := domain.Validate(cmd.Name, cmd.ConfigurationJSON)
	if err != nil {
		return nil, err
	}
	workspaceID := strings.TrimSpace(cmd.WorkspaceID)
	if workspaceID != "" && s.workspaceRepo != nil {
		if _, err := s.workspaceRepo.GetOwned(ctx, workspaceID, cmd.OwnerUserID); err != nil {
			if errors.Is(err, workspace.ErrNotFound) {
				return nil, domain.ErrNotFound
			}
			return nil, err
		}
	}
	now := time.Now().UTC()
	value := &domain.AgentTemplate{ID: uuid.NewString(), WorkspaceID: workspaceID, OwnerUserID: cmd.OwnerUserID, Name: name, ConfigurationJSON: configuration, Status: domain.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.tx.RunInTx(ctx, func(txCtx context.Context) error { return s.repo.Create(txCtx, value) }); err != nil {
		return nil, err
	}
	return value, nil
}

func (s *Service) List(ctx context.Context, owner, workspaceID string) ([]domain.AgentTemplate, error) {
	if strings.TrimSpace(owner) == "" {
		return nil, fmt.Errorf("owner is required")
	}
	return s.repo.ListOwned(ctx, owner, strings.TrimSpace(workspaceID))
}

func (s *Service) Get(ctx context.Context, id, owner string) (*domain.AgentTemplate, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(owner) == "" {
		return nil, domain.ErrNotFound
	}
	return s.repo.GetOwned(ctx, id, owner)
}

func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.AgentTemplate, error) {
	if cmd.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("expected_version must be positive")
	}
	name, configuration, err := domain.Validate(cmd.Name, cmd.ConfigurationJSON)
	if err != nil {
		return nil, err
	}
	value, err := s.repo.GetOwned(ctx, cmd.TemplateID, cmd.OwnerUserID)
	if err != nil {
		return nil, err
	}
	if value.Status != domain.StatusActive {
		return nil, domain.ErrArchived
	}
	if value.Version != cmd.ExpectedVersion {
		return nil, domain.ErrConflict
	}
	value.Name, value.ConfigurationJSON = name, configuration
	value.Version++
	value.UpdatedAt = time.Now().UTC()
	if err := s.tx.RunInTx(ctx, func(txCtx context.Context) error { return s.repo.Update(txCtx, value, cmd.ExpectedVersion) }); err != nil {
		return nil, err
	}
	return value, nil
}

func (s *Service) Archive(ctx context.Context, cmd ArchiveCommand) (*domain.AgentTemplate, error) {
	if cmd.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("expected_version must be positive")
	}
	value, err := s.repo.GetOwned(ctx, cmd.TemplateID, cmd.OwnerUserID)
	if err != nil {
		return nil, err
	}
	if value.Status != domain.StatusActive {
		return nil, domain.ErrArchived
	}
	if value.Version != cmd.ExpectedVersion {
		return nil, domain.ErrConflict
	}
	value.Status = domain.StatusArchived
	value.Version++
	value.UpdatedAt = time.Now().UTC()
	if err := s.tx.RunInTx(ctx, func(txCtx context.Context) error { return s.repo.Archive(txCtx, value, cmd.ExpectedVersion) }); err != nil {
		return nil, err
	}
	return value, nil
}
