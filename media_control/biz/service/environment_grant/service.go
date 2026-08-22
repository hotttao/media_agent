package environment_grant

import (
	"context"
	"github.com/google/uuid"
	"media_agent/media_control/biz/dal"
	d "media_agent/media_control/biz/domain/environment_grant"
	"time"
)

type Service struct {
	repo d.Repository
	tx   dal.TxRunner
}

func New(r d.Repository, tx dal.TxRunner) *Service { return &Service{repo: r, tx: tx} }
func (s *Service) Create(ctx context.Context, env, sub, typ, perm string) (*d.Grant, error) {
	if !s.repo.EnvironmentExists(ctx, env) {
		return nil, d.ErrNotFound
	}
	sub, typ, perm, e := d.Validate(sub, typ, perm)
	if e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	v := &d.Grant{ID: uuid.NewString(), EnvironmentID: env, SubjectID: sub, SubjectType: typ, Permission: perm, Status: "active", CreatedAt: &now}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Create(tx, v) })
	return v, e
}
func (s *Service) List(ctx context.Context, env string) ([]d.Grant, error) {
	if !s.repo.EnvironmentExists(ctx, env) {
		return nil, d.ErrNotFound
	}
	return s.repo.List(ctx, env)
}
func (s *Service) Revoke(ctx context.Context, id, env string) (*d.Grant, error) {
	v, e := s.repo.Get(ctx, id, env)
	if e != nil {
		return nil, e
	}
	if v.Status == "revoked" {
		return nil, d.ErrRevoked
	}
	now := time.Now().UTC()
	v.Status = "revoked"
	v.RevokedAt = &now
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Revoke(tx, v) })
	return v, e
}
