package batch_run

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"media_agent/media_agent_gateway/biz/dal"
	sessiondomain "media_agent/media_agent_gateway/biz/domain/agent_session"
	domain "media_agent/media_agent_gateway/biz/domain/batch_run"
)

type CreateCommand struct {
	Owner, WorkspaceID, IdempotencyKey string
	Items                              []domain.Item
}

type Service struct {
	repo     domain.Repository
	tx       dal.TxRunner
	sessions sessiondomain.Repository
}

func New(repo domain.Repository, tx dal.TxRunner, sessions sessiondomain.Repository) *Service {
	return &Service{repo: repo, tx: tx, sessions: sessions}
}

func (s *Service) Create(ctx context.Context, command CreateCommand) (*domain.BatchRun, error) {
	command.Owner = strings.TrimSpace(command.Owner)
	if command.Owner == "" {
		return nil, fmt.Errorf("owner is required")
	}
	workspace, key, items, err := domain.Validate(command.WorkspaceID, command.IdempotencyKey, command.Items)
	if err != nil {
		return nil, err
	}
	if existing, findErr := s.repo.FindByKey(ctx, command.Owner, workspace, key); findErr == nil {
		return existing, nil
	} else if !errors.Is(findErr, domain.ErrNotFound) {
		return nil, findErr
	}
	validatedSessions := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, ok := validatedSessions[item.SessionID]; ok {
			continue
		}
		session, sessionErr := s.sessions.GetOwned(ctx, item.SessionID, command.Owner)
		if sessionErr != nil {
			return nil, sessionErr
		}
		if session.WorkspaceID != workspace {
			return nil, domain.ErrForbidden
		}
		validatedSessions[item.SessionID] = struct{}{}
	}
	now := time.Now().UTC()
	for i := range items {
		items[i].Status, items[i].CreatedAt, items[i].UpdatedAt = domain.ItemPending, now, now
	}
	value := &domain.BatchRun{ID: uuid.NewString(), WorkspaceID: workspace, RequestedBy: command.Owner, IdempotencyKey: key, Status: domain.StatusCreating, ItemCount: int32(len(items)), Items: items, CreatedAt: now, UpdatedAt: now}
	err = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.Create(tx, value) })
	if errors.Is(err, domain.ErrConflict) {
		return s.repo.FindByKey(ctx, command.Owner, workspace, key)
	}
	return value, err
}

func (s *Service) List(ctx context.Context, owner, workspace, status string) ([]domain.BatchRun, error) {
	parsed, err := domain.ParseStatus(status)
	if err != nil {
		return nil, err
	}
	return s.repo.ListOwned(ctx, owner, strings.TrimSpace(workspace), parsed)
}

func (s *Service) Get(ctx context.Context, id, owner string) (*domain.BatchRun, error) {
	return s.repo.GetOwned(ctx, strings.TrimSpace(id), owner)
}

func (s *Service) Cancel(ctx context.Context, id, owner string) (*domain.BatchRun, error) {
	value, err := s.repo.GetOwned(ctx, strings.TrimSpace(id), owner)
	if err != nil {
		return nil, err
	}
	if value.Status == domain.StatusCancelled {
		return value, nil
	}
	if value.Status != domain.StatusCreating || value.NextCursor != 0 {
		return nil, domain.ErrConflict
	}
	now := time.Now().UTC()
	value.Status, value.CancelledCount, value.FinishedAt, value.UpdatedAt = domain.StatusCancelled, value.ItemCount, &now, now
	for i := range value.Items {
		value.Items[i].Status, value.Items[i].UpdatedAt = domain.ItemCancelled, now
	}
	err = s.tx.RunInTx(ctx, func(tx context.Context) error { return s.repo.CancelCreating(tx, value) })
	return value, err
}
