package share_metadata

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"media_agent/media_control/biz/dal"
	d "media_agent/media_control/biz/domain/share_metadata"
	w "media_agent/media_control/biz/domain/workspace"
	"time"
)

type CreateCommand struct{ Owner, WorkspaceID, ResourceType, ResourceID string }
type Service struct {
	repo d.Repository
	tx   dal.TxRunner
	work w.Repository
}

func New(r d.Repository, tx dal.TxRunner, work w.Repository) *Service {
	return &Service{repo: r, tx: tx, work: work}
}
func (s *Service) Create(ctx context.Context, c CreateCommand) (*d.Share, error) {
	if c.Owner == "" {
		return nil, fmt.Errorf("owner is required")
	}
	if _, e := s.work.GetOwned(ctx, c.WorkspaceID, c.Owner); e != nil {
		return nil, e
	}
	rt, rid, e := d.Validate(c.ResourceType, c.ResourceID)
	if e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	v := &d.Share{ID: uuid.NewString(), WorkspaceID: c.WorkspaceID, ResourceType: rt, ResourceID: rid, OwnerUserID: c.Owner, Status: d.StatusActive, CreatedAt: &now}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Create(tx, v) })
	return v, e
}
func (s *Service) List(ctx context.Context, o, w, rt, rid string) ([]d.Share, error) {
	if _, e := s.work.GetOwned(ctx, w, o); e != nil {
		return nil, e
	}
	return s.repo.ListOwned(ctx, o, w, rt, rid)
}
func (s *Service) Revoke(ctx context.Context, id, o string) (*d.Share, error) {
	v, e := s.repo.GetOwned(ctx, id, o)
	if e != nil {
		return nil, e
	}
	if v.Status == d.StatusRevoked {
		return nil, d.ErrRevoked
	}
	now := time.Now().UTC()
	v.Status = d.StatusRevoked
	v.RevokedAt = &now
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Revoke(tx, v) })
	return v, e
}
