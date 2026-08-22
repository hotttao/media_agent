package memory_store

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"media_agent/media_control/biz/dal"
	credential "media_agent/media_control/biz/domain/credential_reference"
	domain "media_agent/media_control/biz/domain/memory_store"
	workspace "media_agent/media_control/biz/domain/workspace"
	"strings"
	"time"
)

type CreateCommand struct{ OwnerUserID, WorkspaceID, Name, StoreType, ConfigurationJSON, CredentialReferenceID string }
type UpdateCommand struct {
	StoreID, OwnerUserID, Name, StoreType, ConfigurationJSON, CredentialReferenceID string
	ExpectedVersion                                                                 int64
}
type StatusCommand struct {
	StoreID, OwnerUserID, Status string
	ExpectedVersion              int64
}
type Service struct {
	repo           domain.Repository
	workspaceRepo  workspace.Repository
	credentialRepo credential.Repository
	tx             dal.TxRunner
}

func New(repo domain.Repository, tx dal.TxRunner, workspaceRepo workspace.Repository, credentialRepo credential.Repository) *Service {
	return &Service{repo: repo, tx: tx, workspaceRepo: workspaceRepo, credentialRepo: credentialRepo}
}
func (s *Service) validateRefs(ctx context.Context, owner, workspaceID, credentialID string) error {
	if workspaceID == "" {
		return errors.New("workspace_id is required")
	}
	if s.workspaceRepo != nil {
		if _, err := s.workspaceRepo.GetOwned(ctx, workspaceID, owner); err != nil {
			if errors.Is(err, workspace.ErrNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
	}
	if credentialID != "" && s.credentialRepo != nil {
		ref, err := s.credentialRepo.GetOwned(ctx, credentialID, owner)
		if err != nil {
			return err
		}
		if ref.WorkspaceID != workspaceID {
			return domain.ErrNotFound
		}
	}
	return nil
}
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.MemoryStore, error) {
	if strings.TrimSpace(cmd.OwnerUserID) == "" {
		return nil, fmt.Errorf("owner is required")
	}
	name, typ, config, cred, err := domain.Validate(cmd.Name, cmd.StoreType, cmd.ConfigurationJSON, cmd.CredentialReferenceID)
	if err != nil {
		return nil, err
	}
	ws := strings.TrimSpace(cmd.WorkspaceID)
	if err := s.validateRefs(ctx, cmd.OwnerUserID, ws, cred); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	v := &domain.MemoryStore{ID: uuid.NewString(), WorkspaceID: ws, OwnerUserID: cmd.OwnerUserID, Name: name, StoreType: typ, ConfigurationJSON: config, CredentialReferenceID: cred, Status: domain.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Create(tx, v) }); err != nil {
		return nil, err
	}
	return v, nil
}
func (s *Service) List(ctx context.Context, owner, workspaceID string) ([]domain.MemoryStore, error) {
	if strings.TrimSpace(owner) == "" {
		return nil, fmt.Errorf("owner is required")
	}
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	return s.repo.ListOwned(ctx, owner, workspaceID)
}
func (s *Service) Get(ctx context.Context, id, owner string) (*domain.MemoryStore, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(owner) == "" {
		return nil, domain.ErrNotFound
	}
	return s.repo.GetOwned(ctx, id, owner)
}
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.MemoryStore, error) {
	if cmd.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("expected_version must be positive")
	}
	name, typ, config, cred, err := domain.Validate(cmd.Name, cmd.StoreType, cmd.ConfigurationJSON, cmd.CredentialReferenceID)
	if err != nil {
		return nil, err
	}
	v, err := s.repo.GetOwned(ctx, cmd.StoreID, cmd.OwnerUserID)
	if err != nil {
		return nil, err
	}
	if v.Version != cmd.ExpectedVersion {
		return nil, domain.ErrConflict
	}
	if err := s.validateRefs(ctx, cmd.OwnerUserID, v.WorkspaceID, cred); err != nil {
		return nil, err
	}
	v.Name, v.StoreType, v.ConfigurationJSON, v.CredentialReferenceID = name, typ, config, cred
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	if err := s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Update(tx, v, cmd.ExpectedVersion) }); err != nil {
		return nil, err
	}
	return v, nil
}
func (s *Service) SetStatus(ctx context.Context, cmd StatusCommand) (*domain.MemoryStore, error) {
	if cmd.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("expected_version must be positive")
	}
	status, err := domain.ParseStatus(cmd.Status)
	if err != nil {
		return nil, err
	}
	v, err := s.repo.GetOwned(ctx, cmd.StoreID, cmd.OwnerUserID)
	if err != nil {
		return nil, err
	}
	if v.Version != cmd.ExpectedVersion {
		return nil, domain.ErrConflict
	}
	if v.Status == status {
		return v, nil
	}
	v.Status = status
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	if err := s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.SetStatus(tx, v, cmd.ExpectedVersion) }); err != nil {
		return nil, err
	}
	return v, nil
}
