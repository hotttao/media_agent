package schedule

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"media_agent/media_control/biz/dal"
	d "media_agent/media_control/biz/domain/schedule"
	w "media_agent/media_control/biz/domain/workspace"
	"strings"
	"time"
)

type CreateCommand struct{ Owner, WorkspaceID, AgentConfigID, Rule, Timezone string }
type UpdateCommand struct {
	ID, Owner, Rule, Timezone, Status string
	ExpectedVersion                   int64
}
type Service struct {
	repo d.Repository
	tx   dal.TxRunner
	work w.Repository
}

func New(r d.Repository, tx dal.TxRunner, work w.Repository) *Service {
	return &Service{repo: r, tx: tx, work: work}
}
func (s *Service) Create(ctx context.Context, c CreateCommand) (*d.Schedule, error) {
	if c.Owner == "" {
		return nil, fmt.Errorf("owner is required")
	}
	rule, tz, e := d.Validate(c.Rule, c.Timezone)
	if e != nil {
		return nil, e
	}
	if _, e = s.work.GetOwned(ctx, c.WorkspaceID, c.Owner); e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	v := &d.Schedule{ID: uuid.NewString(), WorkspaceID: c.WorkspaceID, RequestedBy: c.Owner, AgentConfigID: c.AgentConfigID, Rule: rule, Timezone: tz, Status: d.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Create(tx, v) })
	return v, e
}
func (s *Service) List(ctx context.Context, o, wid string) ([]d.Schedule, error) {
	return s.repo.ListOwned(ctx, o, wid)
}
func (s *Service) Get(ctx context.Context, id, o string) (*d.Schedule, error) {
	return s.repo.GetOwned(ctx, id, o)
}
func (s *Service) Update(ctx context.Context, c UpdateCommand) (*d.Schedule, error) {
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
	v.Rule, v.Timezone, e = d.Validate(c.Rule, c.Timezone)
	if e != nil {
		return nil, e
	}
	if c.Status != "" {
		v.Status = d.Status(strings.TrimSpace(c.Status))
	}
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Update(tx, v, c.ExpectedVersion) })
	return v, e
}
func (s *Service) Delete(ctx context.Context, id, o string, ev int64) (*d.Schedule, error) {
	v, e := s.Get(ctx, id, o)
	if e != nil {
		return nil, e
	}
	if v.Version != ev {
		return nil, d.ErrConflict
	}
	v.Status = d.StatusDeleted
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Delete(tx, v, ev) })
	return v, e
}
func (s *Service) RunNow(ctx context.Context, id, o string) (*d.Trigger, error) {
	v, e := s.Get(ctx, id, o)
	if e != nil {
		return nil, e
	}
	if v.Status != d.StatusActive {
		return nil, fmt.Errorf("schedule is not active")
	}
	now := time.Now().UTC()
	t := &d.Trigger{ID: uuid.NewString(), ScheduleID: v.ID, DueAt: now, Status: "pending", CreatedAt: now}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.CreateTrigger(tx, t) })
	return t, e
}
