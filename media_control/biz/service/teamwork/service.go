package teamwork

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"media_agent/media_control/biz/dal"
	d "media_agent/media_control/biz/domain/teamwork"
	w "media_agent/media_control/biz/domain/workspace"
	"time"
)

type CreateCommand struct{ Owner, WorkspaceID, Name, EntryMemberID, MembersJSON, EdgesJSON string }
type UpdateCommand struct {
	ID, Owner, Name, EntryMemberID, MembersJSON, EdgesJSON string
	ExpectedVersion                                        int64
}
type Service struct {
	repo d.Repository
	tx   dal.TxRunner
	work w.Repository
}

func New(r d.Repository, tx dal.TxRunner, work w.Repository) *Service {
	return &Service{repo: r, tx: tx, work: work}
}
func (s *Service) Create(ctx context.Context, c CreateCommand) (*d.Topology, error) {
	if c.Owner == "" {
		return nil, fmt.Errorf("owner is required")
	}
	workspace, e := s.work.GetOwned(ctx, c.WorkspaceID, c.Owner)
	if e != nil {
		return nil, e
	}
	name, entry, ms, es, e := d.Validate(c.Name, c.EntryMemberID, c.MembersJSON, c.EdgesJSON)
	if e != nil {
		return nil, e
	}
	if e = validateMemberConfigurations(ms, workspace); e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	v := &d.Topology{ID: uuid.NewString(), WorkspaceID: c.WorkspaceID, Name: name, EntryMemberID: entry, Members: ms, Edges: es, Version: 1, Status: d.StatusActive, CreatedAt: now, UpdatedAt: now}
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Create(tx, v) })
	return v, e
}
func (s *Service) List(ctx context.Context, o, wid string) ([]d.Topology, error) {
	if _, e := s.work.GetOwned(ctx, wid, o); e != nil {
		return nil, e
	}
	return s.repo.ListOwned(ctx, o, wid)
}
func (s *Service) Get(ctx context.Context, id, o string) (*d.Topology, error) {
	return s.repo.GetOwned(ctx, id, o)
}
func (s *Service) Update(ctx context.Context, c UpdateCommand) (*d.Topology, error) {
	if c.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("expected_version must be positive")
	}
	v, e := s.repo.GetOwned(ctx, c.ID, c.Owner)
	if e != nil {
		return nil, e
	}
	if v.Status != d.StatusActive {
		return nil, fmt.Errorf("topology is not active")
	}
	if v.Version != c.ExpectedVersion {
		return nil, d.ErrConflict
	}
	name, entry, ms, es, e := d.Validate(c.Name, c.EntryMemberID, c.MembersJSON, c.EdgesJSON)
	if e != nil {
		return nil, e
	}
	workspace, e := s.work.GetOwned(ctx, v.WorkspaceID, c.Owner)
	if e != nil {
		return nil, e
	}
	if e = validateMemberConfigurations(ms, workspace); e != nil {
		return nil, e
	}
	v.Name, v.EntryMemberID, v.Members, v.Edges = name, entry, ms, es
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Update(tx, v, c.ExpectedVersion) })
	return v, e
}

func (s *Service) RuntimeContext(ctx context.Context, id, owner string) (*d.Topology, error) {
	v, err := s.repo.GetOwned(ctx, id, owner)
	if err != nil {
		return nil, err
	}
	if v.Status != d.StatusActive {
		return nil, d.ErrNotFound
	}
	workspace, err := s.work.GetOwned(ctx, v.WorkspaceID, owner)
	if err != nil {
		return nil, err
	}
	if err = validateMemberConfigurations(v.Members, workspace); err != nil {
		return nil, err
	}
	return v, nil
}

func validateMemberConfigurations(members []d.Member, workspace *w.ConfiguredWorkspace) error {
	for _, member := range members {
		if member.AgentConfigID != workspace.AgentConfiguration.ID {
			return fmt.Errorf("member %q references an agent configuration outside the workspace", member.MemberID)
		}
	}
	return nil
}
func (s *Service) Delete(ctx context.Context, id, o string, ev int64) (*d.Topology, error) {
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
	v.Status = d.StatusDeleted
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	e = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Delete(tx, v, ev) })
	return v, e
}
