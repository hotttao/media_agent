package afs_binding

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"media_agent/media_control/biz/dal"
	d "media_agent/media_control/biz/domain/afs_binding"
	w "media_agent/media_control/biz/domain/workspace"
	"strings"
	"time"
)

type CreateCommand struct{ Owner, WorkspaceID, ExternalRef, DesiredState string }
type MemberCommand struct{ Owner, BindingID, SubjectID, SubjectType, ExternalMemberRef string }
type Service struct {
	repo d.Repository
	tx   dal.TxRunner
	work w.Repository
}

func New(r d.Repository, tx dal.TxRunner, work w.Repository) *Service {
	return &Service{repo: r, tx: tx, work: work}
}
func (s *Service) Create(ctx context.Context, c CreateCommand) (*d.Binding, error) {
	if _, e := s.work.GetOwned(ctx, c.WorkspaceID, c.Owner); e != nil {
		return nil, e
	}
	ref, state, e := d.Validate(c.ExternalRef, c.DesiredState)
	if e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	v := &d.Binding{ID: uuid.NewString(), WorkspaceID: c.WorkspaceID, ExternalRef: ref, DesiredState: state, ObservedState: "unknown", Version: 1, CreatedAt: now, UpdatedAt: now}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Create(tx, v) })
	return v, e
}
func (s *Service) List(ctx context.Context, o, wid string) ([]d.Binding, error) {
	if _, e := s.work.GetOwned(ctx, wid, o); e != nil {
		return nil, e
	}
	return s.repo.ListOwned(ctx, o, wid)
}
func (s *Service) Delete(ctx context.Context, id, o string) (*d.Binding, error) {
	v, e := s.repo.GetOwned(ctx, id, o)
	if e != nil {
		return nil, e
	}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Delete(tx, v) })
	return v, e
}
func (s *Service) Members(ctx context.Context, id, o string) ([]d.Member, error) {
	if _, e := s.repo.GetOwned(ctx, id, o); e != nil {
		return nil, e
	}
	return s.repo.ListMembers(ctx, id)
}
func (s *Service) AddMember(ctx context.Context, c MemberCommand) (*d.Member, error) {
	v, e := s.repo.GetOwned(ctx, c.BindingID, c.Owner)
	if e != nil {
		return nil, e
	}
	id, typ, ext, e := d.ValidateMember(c.SubjectID, c.SubjectType, c.ExternalMemberRef)
	if e != nil {
		return nil, e
	}
	m := &d.Member{BindingID: v.ID, SubjectID: id, SubjectType: typ, ExternalMemberRef: ext, Status: "pending", UpdatedAt: time.Now().UTC()}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.AddMember(tx, m) })
	return m, e
}
func (s *Service) RemoveMember(ctx context.Context, id, o, subject string) (*d.Member, error) {
	v, e := s.repo.GetOwned(ctx, id, o)
	if e != nil {
		return nil, e
	}
	if strings.TrimSpace(subject) == "" {
		return nil, fmt.Errorf("subject_id is required")
	}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.RemoveMember(tx, v.ID, subject) })
	return &d.Member{BindingID: v.ID, SubjectID: subject, Status: "removed", UpdatedAt: time.Now().UTC()}, e
}
func (s *Service) Reconcile(ctx context.Context, id, o, observed string) (*d.Binding, error) {
	v, e := s.repo.GetOwned(ctx, id, o)
	if e != nil {
		return nil, e
	}
	observed = strings.TrimSpace(observed)
	if observed == "" {
		return nil, fmt.Errorf("observed_state is required")
	}
	v.ObservedState = observed
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Reconcile(tx, v, observed) })
	return v, e
}
