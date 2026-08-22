package platform_administration

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "media_agent/media_control/biz/domain/platform_administration"
)

type fakeRepository struct {
	settings map[string]*domain.Setting
	events   []domain.ChangeEvent
}

func (r *fakeRepository) Create(_ context.Context, value *domain.Setting) error {
	if r.settings[value.Key] != nil {
		return domain.ErrConflict
	}
	copy := *value
	r.settings[value.Key] = &copy
	return nil
}
func (r *fakeRepository) List(context.Context) ([]domain.Setting, error) {
	result := make([]domain.Setting, 0, len(r.settings))
	for _, value := range r.settings {
		result = append(result, *value)
	}
	return result, nil
}
func (r *fakeRepository) Get(_ context.Context, key string) (*domain.Setting, error) {
	value := r.settings[key]
	if value == nil {
		return nil, domain.ErrNotFound
	}
	copy := *value
	return &copy, nil
}
func (r *fakeRepository) Update(_ context.Context, value *domain.Setting, expected int64) error {
	current := r.settings[value.Key]
	if current == nil || current.Version != expected {
		return domain.ErrConflict
	}
	copy := *value
	r.settings[value.Key] = &copy
	return nil
}
func (r *fakeRepository) Enqueue(_ context.Context, event *domain.ChangeEvent) error {
	r.events = append(r.events, *event)
	return nil
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func TestPlatformSettingLifecycleAndOutbox(t *testing.T) {
	repository := &fakeRepository{settings: map[string]*domain.Setting{}}
	service := New(repository, directTx{})
	service.now = func() time.Time { return time.Unix(100, 0).UTC() }
	admin := domain.Actor{UserID: "admin-1", Role: "admin"}

	created, err := service.Create(context.Background(), admin, "runtime.defaults", ` {"cpu": 2} `)
	if err != nil || created.Version != 1 || created.ValueJSON != `{"cpu":2}` {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	if len(repository.events) != 1 || repository.events[0].Version != 1 {
		t.Fatalf("create outbox=%+v", repository.events)
	}
	updated, err := service.Update(context.Background(), admin, created.Key, `{"cpu":4}`, 1)
	if err != nil || updated.Version != 2 || updated.UpdatedBy != "admin-1" {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	if len(repository.events) != 2 || repository.events[1].Version != 2 || repository.events[1].EventType != "platform_setting.changed" {
		t.Fatalf("update outbox=%+v", repository.events)
	}
	if _, err := service.Update(context.Background(), admin, created.Key, `{}`, 1); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale update accepted: %v", err)
	}
}

func TestPlatformSettingRejectsOrdinaryUser(t *testing.T) {
	service := New(&fakeRepository{settings: map[string]*domain.Setting{}}, directTx{})
	user := domain.Actor{UserID: "user-1", Role: "user"}
	if _, err := service.Create(context.Background(), user, "runtime.defaults", `{}`); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ordinary user create: %v", err)
	}
	if _, err := service.List(context.Background(), user); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ordinary user list: %v", err)
	}
}
