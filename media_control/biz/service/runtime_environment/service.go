package runtime_environment

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"media_agent/media_control/biz/dal"
	d "media_agent/media_control/biz/domain/runtime_environment"
	w "media_agent/media_control/biz/domain/workspace"
	"path/filepath"
	"strings"
	"time"
)

type CreateCommand struct{ Name, Endpoint, CapacityJSON string }
type UpdateCommand struct {
	ID, Name, Endpoint, CapacityJSON, Status string
	ExpectedVersion                          int64
}
type Service struct {
	repo d.Repository
	tx   dal.TxRunner
	work w.Repository
}

func New(repo d.Repository, tx dal.TxRunner, work w.Repository) *Service {
	return &Service{repo: repo, tx: tx, work: work}
}
func (s *Service) Create(ctx context.Context, c CreateCommand) (*d.Environment, error) {
	name, endpoint, capacity, e := d.Validate(c.Name, c.Endpoint, c.CapacityJSON)
	if e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	v := &d.Environment{ID: uuid.NewString(), Name: name, Endpoint: endpoint, CapacityJSON: capacity, Status: d.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.CreateEnvironment(tx, v) })
	return v, e
}
func (s *Service) List(ctx context.Context) ([]d.Environment, error) {
	return s.repo.ListEnvironments(ctx)
}
func (s *Service) Get(ctx context.Context, id string) (*d.Environment, error) {
	return s.repo.GetEnvironment(ctx, id)
}
func (s *Service) Update(ctx context.Context, c UpdateCommand) (*d.Environment, error) {
	if c.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("expected_version must be positive")
	}
	v, e := s.repo.GetEnvironment(ctx, c.ID)
	if e != nil {
		return nil, e
	}
	name, endpoint, capacity, e := d.Validate(c.Name, c.Endpoint, c.CapacityJSON)
	if e != nil {
		return nil, e
	}
	status := v.Status
	if strings.TrimSpace(c.Status) != "" {
		status, e = d.ParseStatus(c.Status)
		if e != nil {
			return nil, e
		}
	}
	if v.Version != c.ExpectedVersion {
		return nil, d.ErrConflict
	}
	v.Name, v.Endpoint, v.CapacityJSON, v.Status = name, endpoint, capacity, status
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.UpdateEnvironment(tx, v, c.ExpectedVersion) })
	return v, e
}
func (s *Service) Delete(ctx context.Context, id string, expected int64) (*d.Environment, error) {
	if expected <= 0 {
		return nil, fmt.Errorf("expected_version must be positive")
	}
	v, e := s.repo.GetEnvironment(ctx, id)
	if e != nil {
		return nil, e
	}
	if v.Version != expected {
		return nil, d.ErrConflict
	}
	v.Status = d.StatusOffline
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.DeleteEnvironment(tx, v, expected) })
	return v, e
}
func (s *Service) Placement(ctx context.Context, workspaceID, owner string) (*d.Placement, error) {
	if _, e := s.work.GetOwned(ctx, workspaceID, owner); e != nil {
		return nil, e
	}
	return s.repo.FindPlacement(ctx, workspaceID)
}
func (s *Service) Reconcile(ctx context.Context, workspaceID, owner string) (*d.Placement, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if _, e := s.work.GetOwned(ctx, workspaceID, owner); e != nil {
		return nil, e
	}
	envs, e := s.repo.ListEnvironments(ctx)
	if e != nil {
		return nil, e
	}
	var selected *d.Environment
	for i := range envs {
		if envs[i].Status == d.StatusActive {
			selected = &envs[i]
			break
		}
	}
	current, e := s.repo.FindPlacement(ctx, workspaceID)
	if e != nil && e != d.ErrPlacementNotFound {
		return nil, e
	}
	now := time.Now().UTC()
	v := &d.Placement{WorkspaceID: workspaceID, Version: 1, UpdatedAt: now}
	expected := int64(0)
	if current != nil {
		v.Version = current.Version + 1
		expected = current.Version
	}
	if selected == nil {
		v.Status = d.PlacementUnplaced
		v.Reason = "no active runtime environment"
	} else {
		v.Status = d.PlacementPlaced
		v.EnvironmentID = selected.ID
		v.VolumeRef = filepath.Join("/workspace", workspaceID)
	}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.UpsertPlacement(tx, v, expected) })
	return v, e
}
func (s *Service) EnsureRuntimeDirectory(ctx context.Context, workspaceID, owner string) (*d.Placement, error) {
	v, e := s.Reconcile(ctx, workspaceID, owner)
	if e != nil {
		return nil, e
	}
	if v.Status != d.PlacementPlaced {
		return v, fmt.Errorf("workspace is not placed: %s", v.Reason)
	}
	return v, nil
}
