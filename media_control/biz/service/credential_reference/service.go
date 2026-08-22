package credential_reference

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"media_agent/media_control/biz/dal"
	domain "media_agent/media_control/biz/domain/credential_reference"
	provider "media_agent/media_control/biz/domain/provider"
	workspace "media_agent/media_control/biz/domain/workspace"
	"strings"
	"time"
)

type CreateCommand struct{ OwnerUserID, WorkspaceID, Name, ProviderID, SecretRef string }
type UpdateCommand struct {
	CredentialID, OwnerUserID, Name, ProviderID, SecretRef string
	ExpectedVersion                                        int64
}
type RevokeCommand struct {
	CredentialID, OwnerUserID string
	ExpectedVersion           int64
}
type Service struct {
	repo          domain.Repository
	workspaceRepo workspace.Repository
	providerRepo  provider.Repository
	tx            dal.TxRunner
}

func New(repo domain.Repository, tx dal.TxRunner, workspaceRepo workspace.Repository, providerRepo provider.Repository) *Service {
	return &Service{repo: repo, tx: tx, workspaceRepo: workspaceRepo, providerRepo: providerRepo}
}
func (s *Service) validateRefs(ctx context.Context, owner, workspaceID, providerID string) error {
	if s.providerRepo != nil {
		if _, err := s.providerRepo.Get(ctx, providerID); err != nil {
			return err
		}
	}
	if workspaceID != "" && s.workspaceRepo != nil {
		if _, err := s.workspaceRepo.GetOwned(ctx, workspaceID, owner); err != nil {
			if errors.Is(err, workspace.ErrNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
	}
	return nil
}
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.CredentialReference, error) {
	if strings.TrimSpace(cmd.OwnerUserID) == "" {
		return nil, fmt.Errorf("owner is required")
	}
	name, pid, secret, err := domain.Validate(cmd.Name, cmd.ProviderID, cmd.SecretRef)
	if err != nil {
		return nil, err
	}
	ws := strings.TrimSpace(cmd.WorkspaceID)
	if err := s.validateRefs(ctx, cmd.OwnerUserID, ws, pid); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	v := &domain.CredentialReference{ID: uuid.NewString(), WorkspaceID: ws, OwnerUserID: cmd.OwnerUserID, Name: name, ProviderID: pid, Status: domain.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Create(tx, v, secret) }); err != nil {
		return nil, err
	}
	return v, nil
}
func (s *Service) List(ctx context.Context, owner, workspaceID string) ([]domain.CredentialReference, error) {
	if strings.TrimSpace(owner) == "" {
		return nil, fmt.Errorf("owner is required")
	}
	return s.repo.ListOwned(ctx, owner, strings.TrimSpace(workspaceID))
}
func (s *Service) Get(ctx context.Context, id, owner string) (*domain.CredentialReference, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(owner) == "" {
		return nil, domain.ErrNotFound
	}
	return s.repo.GetOwned(ctx, id, owner)
}
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.CredentialReference, error) {
	if cmd.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("expected_version must be positive")
	}
	name, pid, secret, err := domain.Validate(cmd.Name, cmd.ProviderID, cmd.SecretRef)
	if err != nil {
		return nil, err
	}
	v, err := s.repo.GetOwned(ctx, cmd.CredentialID, cmd.OwnerUserID)
	if err != nil {
		return nil, err
	}
	if v.Status == domain.StatusRevoked {
		return nil, domain.ErrRevoked
	}
	if v.Version != cmd.ExpectedVersion {
		return nil, domain.ErrConflict
	}
	if err := s.validateRefs(ctx, cmd.OwnerUserID, v.WorkspaceID, pid); err != nil {
		return nil, err
	}
	v.Name, v.ProviderID = name, pid
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	if err := s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Update(tx, v, secret, cmd.ExpectedVersion) }); err != nil {
		return nil, err
	}
	return v, nil
}
func (s *Service) Revoke(ctx context.Context, cmd RevokeCommand) (*domain.CredentialReference, error) {
	if cmd.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("expected_version must be positive")
	}
	v, err := s.repo.GetOwned(ctx, cmd.CredentialID, cmd.OwnerUserID)
	if err != nil {
		return nil, err
	}
	if v.Status == domain.StatusRevoked {
		return nil, domain.ErrRevoked
	}
	if v.Version != cmd.ExpectedVersion {
		return nil, domain.ErrConflict
	}
	v.Status = domain.StatusRevoked
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	if err := s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Revoke(tx, v, cmd.ExpectedVersion) }); err != nil {
		return nil, err
	}
	return v, nil
}
