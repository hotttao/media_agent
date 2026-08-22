package public_export

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"media_agent/media_control/biz/dal"
	d "media_agent/media_control/biz/domain/public_export"
	w "media_agent/media_control/biz/domain/workspace"
	"time"
)

type CreateCommand struct{ Owner, WorkspaceID, ResourceType, ResourceID, ObjectPath string }
type Service struct {
	repo d.Repository
	tx   dal.TxRunner
	work w.Repository
}

func New(r d.Repository, tx dal.TxRunner, work w.Repository) *Service {
	return &Service{repo: r, tx: tx, work: work}
}
func (s *Service) Create(ctx context.Context, c CreateCommand) (*d.Export, error) {
	if c.Owner == "" {
		return nil, fmt.Errorf("owner is required")
	}
	if _, e := s.work.GetOwned(ctx, c.WorkspaceID, c.Owner); e != nil {
		return nil, e
	}
	rt, rid, path, e := d.Validate(c.ResourceType, c.ResourceID, c.ObjectPath)
	if e != nil {
		return nil, e
	}
	v := &d.Export{ID: uuid.NewString(), WorkspaceID: c.WorkspaceID, ResourceType: rt, ResourceID: rid, ObjectPath: path, Status: d.StatusPending, CreatedBy: c.Owner, CreatedAt: time.Now().UTC()}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Create(tx, v) })
	return v, e
}
func (s *Service) List(ctx context.Context, o, w, rt, rid string) ([]d.Export, error) {
	if _, e := s.work.GetOwned(ctx, w, o); e != nil {
		return nil, e
	}
	return s.repo.ListOwned(ctx, o, w, rt, rid)
}
func (s *Service) SetStatus(ctx context.Context, id, o, target string) (*d.Export, error) {
	v, e := s.repo.GetOwned(ctx, id, o)
	if e != nil {
		return nil, e
	}
	next, e := d.ParseStatus(target)
	if e != nil {
		return nil, e
	}
	if !d.CanTransition(v.Status, next) {
		return nil, d.ErrTransition
	}
	from := v.Status
	v.Status = next
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.SetStatus(tx, v, string(from)) })
	return v, e
}
