package workspace_metadata

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"media_agent/media_control/biz/dal"
	w "media_agent/media_control/biz/domain/workspace"
	d "media_agent/media_control/biz/domain/workspace_metadata"
	"time"
)

type CreateCommand struct{ Owner, WorkspaceID, Name, Color string }
type UpdateCommand struct {
	ID, Owner, Name, Color string
	ExpectedVersion        int64
}
type AssociationCommand struct{ Owner, TagID, WorkspaceID, ResourceType, ResourceID string }
type Service struct {
	repo d.Repository
	tx   dal.TxRunner
	work w.Repository
}

func New(r d.Repository, tx dal.TxRunner, work w.Repository) *Service {
	return &Service{repo: r, tx: tx, work: work}
}
func (s *Service) Create(ctx context.Context, c CreateCommand) (*d.Tag, error) {
	if c.Owner == "" {
		return nil, fmt.Errorf("owner is required")
	}
	if _, e := s.work.GetOwned(ctx, c.WorkspaceID, c.Owner); e != nil {
		return nil, e
	}
	name, color, e := d.Validate(c.Name, c.Color)
	if e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	v := &d.Tag{ID: uuid.NewString(), WorkspaceID: c.WorkspaceID, Name: name, Color: color, CreatedBy: c.Owner, Version: 1, CreatedAt: now, UpdatedAt: now}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Create(tx, v) })
	return v, e
}
func (s *Service) List(ctx context.Context, o, wid string) ([]d.Tag, error) {
	if _, e := s.work.GetOwned(ctx, wid, o); e != nil {
		return nil, e
	}
	return s.repo.ListOwned(ctx, o, wid)
}
func (s *Service) Update(ctx context.Context, c UpdateCommand) (*d.Tag, error) {
	if c.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("expected_version must be positive")
	}
	v, e := s.repo.GetOwned(ctx, c.ID, c.Owner)
	if e != nil {
		return nil, e
	}
	if v.Version != c.ExpectedVersion {
		return nil, d.ErrConflict
	}
	v.Name, v.Color, e = d.Validate(c.Name, c.Color)
	if e != nil {
		return nil, e
	}
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Update(tx, v, c.ExpectedVersion) })
	return v, e
}
func (s *Service) Delete(ctx context.Context, id, o string, ev int64) (*d.Tag, error) {
	if ev <= 0 {
		return nil, fmt.Errorf("expected_version must be positive")
	}
	v, e := s.repo.GetOwned(ctx, id, o)
	if e != nil {
		return nil, e
	}
	if v.Version != ev {
		return nil, d.ErrConflict
	}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Delete(tx, v, ev) })
	return v, e
}
func (s *Service) Attach(ctx context.Context, c AssociationCommand) (*d.ResourceTag, error) {
	if _, e := s.work.GetOwned(ctx, c.WorkspaceID, c.Owner); e != nil {
		return nil, e
	}
	rt, rid, e := d.ValidateResource(c.ResourceType, c.ResourceID)
	if e != nil {
		return nil, e
	}
	tag, e := s.repo.GetOwned(ctx, c.TagID, c.Owner)
	if e != nil {
		return nil, e
	}
	if tag.WorkspaceID != c.WorkspaceID {
		return nil, d.ErrNotFound
	}
	v := &d.ResourceTag{TagID: c.TagID, WorkspaceID: c.WorkspaceID, ResourceType: rt, ResourceID: rid, CreatedAt: time.Now().UTC()}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Attach(tx, v) })
	return v, e
}
func (s *Service) Detach(ctx context.Context, c AssociationCommand) (*d.ResourceTag, error) {
	if _, e := s.work.GetOwned(ctx, c.WorkspaceID, c.Owner); e != nil {
		return nil, e
	}
	rt, rid, e := d.ValidateResource(c.ResourceType, c.ResourceID)
	if e != nil {
		return nil, e
	}
	tag, e := s.repo.GetOwned(ctx, c.TagID, c.Owner)
	if e != nil {
		return nil, e
	}
	if tag.WorkspaceID != c.WorkspaceID {
		return nil, d.ErrNotFound
	}
	v := &d.ResourceTag{TagID: c.TagID, WorkspaceID: c.WorkspaceID, ResourceType: rt, ResourceID: rid}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Detach(tx, v) })
	return v, e
}
func (s *Service) ListResourceTags(ctx context.Context, o, wid, rt, rid string) ([]d.Tag, error) {
	if _, e := s.work.GetOwned(ctx, wid, o); e != nil {
		return nil, e
	}
	return s.repo.ListResourceTags(ctx, o, wid, rt, rid)
}
