package platform_administration

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	domain "media_agent/media_control/biz/domain/platform_administration"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, value *domain.Setting) error {
	err := dal.FromContext(ctx, r.db).Create(toRow(value)).Error
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return domain.ErrConflict
	}
	return err
}
func (r *Repository) List(ctx context.Context) ([]domain.Setting, error) {
	var rows []model.PlatformSetting
	if err := dal.FromContext(ctx, r.db).Order("key ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Setting, 0, len(rows))
	for _, row := range rows {
		result = append(result, toDomain(row))
	}
	return result, nil
}
func (r *Repository) Get(ctx context.Context, key string) (*domain.Setting, error) {
	var row model.PlatformSetting
	err := dal.FromContext(ctx, r.db).Where("key = ?", key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	value := toDomain(row)
	return &value, nil
}
func (r *Repository) Update(ctx context.Context, value *domain.Setting, expectedVersion int64) error {
	result := dal.FromContext(ctx, r.db).Model(&model.PlatformSetting{}).
		Where("key = ? AND version = ?", value.Key, expectedVersion).
		Updates(map[string]any{"value": value.ValueJSON, "version": value.Version, "updated_by": value.UpdatedBy, "updated_at": value.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}
func (r *Repository) Enqueue(ctx context.Context, event *domain.ChangeEvent) error {
	return dal.FromContext(ctx, r.db).Create(&model.PlatformSettingOutbox{ID: event.ID, SettingKey: event.SettingKey, Version: event.Version, EventType: event.EventType, Payload: event.PayloadJSON, CreatedAt: event.CreatedAt}).Error
}
func toRow(value *domain.Setting) *model.PlatformSetting {
	return &model.PlatformSetting{Key: value.Key, Value: value.ValueJSON, Version: value.Version, UpdatedBy: value.UpdatedBy, UpdatedAt: value.UpdatedAt}
}
func toDomain(row model.PlatformSetting) domain.Setting {
	return domain.Setting{Key: row.Key, ValueJSON: row.Value, Version: row.Version, UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt}
}
