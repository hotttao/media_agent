package platform_administration

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"media_agent/media_control/biz/dal"
	domain "media_agent/media_control/biz/domain/platform_administration"
)

type Service struct {
	repo domain.Repository
	tx   dal.TxRunner
	now  func() time.Time
}

func New(repo domain.Repository, tx dal.TxRunner) *Service {
	return &Service{repo: repo, tx: tx, now: func() time.Time { return time.Now().UTC() }}
}
func (s *Service) Create(ctx context.Context, actor domain.Actor, key, valueJSON string) (*domain.Setting, error) {
	if err := actor.ValidateAdmin(); err != nil {
		return nil, err
	}
	key, valueJSON, err := domain.Validate(key, valueJSON)
	if err != nil {
		return nil, err
	}
	now := s.now()
	value := &domain.Setting{Key: key, ValueJSON: valueJSON, Version: 1, UpdatedBy: strings.TrimSpace(actor.UserID), UpdatedAt: now}
	err = s.tx.RunInTx(ctx, func(tx context.Context) error {
		if err := s.repo.Create(tx, value); err != nil {
			return err
		}
		return s.enqueue(tx, value)
	})
	return value, err
}
func (s *Service) List(ctx context.Context, actor domain.Actor) ([]domain.Setting, error) {
	if err := actor.ValidateAdmin(); err != nil {
		return nil, err
	}
	return s.repo.List(ctx)
}
func (s *Service) Get(ctx context.Context, actor domain.Actor, key string) (*domain.Setting, error) {
	if err := actor.ValidateAdmin(); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, strings.TrimSpace(key))
}
func (s *Service) Update(ctx context.Context, actor domain.Actor, key, valueJSON string, expectedVersion int64) (*domain.Setting, error) {
	if err := actor.ValidateAdmin(); err != nil {
		return nil, err
	}
	if expectedVersion <= 0 {
		return nil, fmt.Errorf("expected_version must be positive")
	}
	key, valueJSON, err := domain.Validate(key, valueJSON)
	if err != nil {
		return nil, err
	}
	value, err := s.repo.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if value.Version != expectedVersion {
		return nil, domain.ErrConflict
	}
	value.ValueJSON, value.Version, value.UpdatedBy, value.UpdatedAt = valueJSON, value.Version+1, strings.TrimSpace(actor.UserID), s.now()
	err = s.tx.RunInTx(ctx, func(tx context.Context) error {
		if err := s.repo.Update(tx, value, expectedVersion); err != nil {
			return err
		}
		return s.enqueue(tx, value)
	})
	return value, err
}
func (s *Service) enqueue(ctx context.Context, value *domain.Setting) error {
	return s.repo.Enqueue(ctx, &domain.ChangeEvent{ID: uuid.NewString(), SettingKey: value.Key, Version: value.Version, EventType: "platform_setting.changed", PayloadJSON: fmt.Sprintf(`{"key":%q,"version":%d}`, value.Key, value.Version), CreatedAt: value.UpdatedAt})
}
