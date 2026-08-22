package agent_task

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"media_agent/media_agent_gateway/biz/dal"
	session "media_agent/media_agent_gateway/biz/domain/agent_session"
	domain "media_agent/media_agent_gateway/biz/domain/agent_task"
	"strings"
	"time"
)

type WorkspaceAuthorizer interface {
	AuthorizeWorkspace(context.Context, string, string, string) error
}
type CreateCommand struct{ Owner, AuthToken, WorkspaceID, SessionID, Source, SourceResourceID, IdempotencyKey, InputJSON string }
type Service struct {
	repo       domain.Repository
	outbox     domain.OutboxRepository
	tx         dal.TxRunner
	sessions   session.Repository
	workspaces WorkspaceAuthorizer
}

func New(repo domain.Repository, outbox domain.OutboxRepository, tx dal.TxRunner, sessions session.Repository, workspaces WorkspaceAuthorizer) *Service {
	return &Service{repo: repo, outbox: outbox, tx: tx, sessions: sessions, workspaces: workspaces}
}
func (s *Service) Create(ctx context.Context, c CreateCommand) (*domain.Task, error) {
	if strings.TrimSpace(c.Owner) == "" {
		return nil, fmt.Errorf("owner is required")
	}
	workspace, source, sourceID, key, input, err := domain.Validate(c.WorkspaceID, c.Source, c.SourceResourceID, c.IdempotencyKey, c.InputJSON)
	if err != nil {
		return nil, err
	}
	if existing, e := s.repo.FindByKey(ctx, c.Owner, source, sourceID, key); e == nil {
		return existing, nil
	} else if !errors.Is(e, domain.ErrNotFound) {
		return nil, e
	}
	if c.SessionID != "" {
		value, e := s.sessions.GetOwned(ctx, c.SessionID, c.Owner)
		if e != nil {
			return nil, e
		}
		if value.WorkspaceID != workspace {
			return nil, domain.ErrForbidden
		}
	} else if s.workspaces != nil {
		if e := s.workspaces.AuthorizeWorkspace(ctx, c.Owner, workspace, c.AuthToken); e != nil {
			return nil, e
		}
	}
	now := time.Now().UTC()
	v := &domain.Task{ID: uuid.NewString(), WorkspaceID: workspace, RequestedBy: c.Owner, SessionID: strings.TrimSpace(c.SessionID), Source: source, SourceResourceID: sourceID, IdempotencyKey: key, InputJSON: input, Status: domain.StatusQueued, CreatedAt: now, UpdatedAt: now}
	err = s.tx.RunInTx(ctx, func(tx context.Context) error {
		if err := s.repo.Create(tx, v); err != nil {
			return err
		}
		if s.outbox == nil {
			return nil
		}
		return s.outbox.Enqueue(tx, &domain.OutboxEvent{AggregateID: v.ID, EventType: "agent_task.dispatch", PayloadJSON: fmt.Sprintf(`{"task_id":%q}`, v.ID), AvailableAt: now, CreatedAt: now, UpdatedAt: now})
	})
	if errors.Is(err, domain.ErrConflict) {
		return s.repo.FindByKey(ctx, c.Owner, source, sourceID, key)
	}
	return v, err
}
func (s *Service) List(ctx context.Context, owner, workspace, status string) ([]domain.Task, error) {
	parsed, err := domain.ParseStatus(status)
	if err != nil {
		return nil, err
	}
	return s.repo.ListOwned(ctx, owner, strings.TrimSpace(workspace), parsed)
}
func (s *Service) Get(ctx context.Context, id, owner string) (*domain.Task, error) {
	return s.repo.GetOwned(ctx, id, owner)
}
func (s *Service) Cancel(ctx context.Context, id, owner string) (*domain.Task, error) {
	v, err := s.repo.GetOwned(ctx, id, owner)
	if err != nil {
		return nil, err
	}
	if v.Status == domain.StatusCancelled {
		return v, nil
	}
	if domain.IsTerminal(v.Status) {
		return nil, domain.ErrConflict
	}
	if v.Status != domain.StatusQueued && v.Status != domain.StatusRetryWait && v.Status != domain.StatusRunning && v.Status != domain.StatusWaitingTurn {
		return nil, domain.ErrConflict
	}
	now := time.Now().UTC()
	v.Status = domain.StatusCancelled
	v.FinishedAt = &now
	v.UpdatedAt = now
	from := []domain.Status{domain.StatusQueued, domain.StatusRetryWait, domain.StatusRunning, domain.StatusWaitingTurn}
	err = s.tx.RunInTx(ctx, func(tx context.Context) error {
		if err := s.repo.Cancel(tx, v, from); err != nil {
			return err
		}
		if s.outbox == nil {
			return nil
		}
		if err := s.outbox.Enqueue(tx, &domain.OutboxEvent{AggregateID: v.ID, EventType: "agent_task.cancel", PayloadJSON: fmt.Sprintf(`{"task_id":%q}`, v.ID), AvailableAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		return s.outbox.Enqueue(tx, &domain.OutboxEvent{AggregateID: v.ID, EventType: "agent_task.cancelled", PayloadJSON: fmt.Sprintf(`{"task_id":%q,"status":"cancelled"}`, v.ID), AvailableAt: now, CreatedAt: now, UpdatedAt: now})
	})
	return v, err
}
