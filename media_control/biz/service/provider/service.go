package provider

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"media_agent/media_control/biz/dal"
	domain "media_agent/media_control/biz/domain/provider"
	"strings"
	"time"
)

type CreateCommand struct{ Name, ProviderType, Endpoint, CapabilitiesJSON string }
type UpdateCommand struct {
	ProviderID, Name, ProviderType, Endpoint, CapabilitiesJSON string
	ExpectedVersion                                            int64
}
type StatusCommand struct {
	ProviderID, Status string
	ExpectedVersion    int64
}
type Service struct {
	repo domain.Repository
	tx   dal.TxRunner
}

func New(repo domain.Repository, tx dal.TxRunner) *Service { return &Service{repo: repo, tx: tx} }
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.Provider, error) {
	n, t, e, c, err := domain.Validate(cmd.Name, cmd.ProviderType, cmd.Endpoint, cmd.CapabilitiesJSON)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	v := &domain.Provider{ID: uuid.NewString(), Name: n, ProviderType: t, Endpoint: e, CapabilitiesJSON: c, Status: domain.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Create(tx, v) }); err != nil {
		return nil, err
	}
	return v, nil
}
func (s *Service) List(ctx context.Context) ([]domain.Provider, error) { return s.repo.List(ctx) }
func (s *Service) Get(ctx context.Context, id string) (*domain.Provider, error) {
	if strings.TrimSpace(id) == "" {
		return nil, domain.ErrNotFound
	}
	return s.repo.Get(ctx, id)
}
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.Provider, error) {
	if cmd.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("expected_version must be positive")
	}
	n, t, e, c, err := domain.Validate(cmd.Name, cmd.ProviderType, cmd.Endpoint, cmd.CapabilitiesJSON)
	if err != nil {
		return nil, err
	}
	v, err := s.repo.Get(ctx, cmd.ProviderID)
	if err != nil {
		return nil, err
	}
	if v.Version != cmd.ExpectedVersion {
		return nil, domain.ErrConflict
	}
	v.Name, v.ProviderType, v.Endpoint, v.CapabilitiesJSON = n, t, e, c
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	if err := s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Update(tx, v, cmd.ExpectedVersion) }); err != nil {
		return nil, err
	}
	return v, nil
}
func (s *Service) SetStatus(ctx context.Context, cmd StatusCommand) (*domain.Provider, error) {
	if cmd.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("expected_version must be positive")
	}
	st, err := domain.ParseStatus(cmd.Status)
	if err != nil {
		return nil, err
	}
	v, err := s.repo.Get(ctx, cmd.ProviderID)
	if err != nil {
		return nil, err
	}
	if v.Version != cmd.ExpectedVersion {
		return nil, domain.ErrConflict
	}
	if v.Status == st {
		return v, nil
	}
	v.Status = st
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	if err := s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.SetStatus(tx, v, cmd.ExpectedVersion) }); err != nil {
		return nil, err
	}
	return v, nil
}
