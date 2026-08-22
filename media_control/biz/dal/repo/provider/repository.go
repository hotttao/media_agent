package provider

import (
	"context"
	"errors"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	domain "media_agent/media_control/biz/domain/provider"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Create(ctx context.Context, value *domain.Provider) error {
	return dal.FromContext(ctx, r.db).Table("providers").Create(map[string]any{"id": value.ID, "name": value.Name, "provider_type": value.ProviderType, "endpoint": value.Endpoint, "capabilities": datatypes.JSON([]byte(value.CapabilitiesJSON)), "status": string(value.Status), "version": value.Version, "created_at": value.CreatedAt, "updated_at": value.UpdatedAt}).Error
}
func (r *Repository) List(ctx context.Context) ([]domain.Provider, error) {
	var rows []model.Provider
	if err := dal.FromContext(ctx, r.db).Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Provider, 0, len(rows))
	for i := range rows {
		out = append(out, fromRow(&rows[i]))
	}
	return out, nil
}
func (r *Repository) Get(ctx context.Context, id string) (*domain.Provider, error) {
	var row model.Provider
	err := dal.FromContext(ctx, r.db).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v := fromRow(&row)
	return &v, nil
}
func (r *Repository) Update(ctx context.Context, v *domain.Provider, expected int64) error {
	res := dal.FromContext(ctx, r.db).Model(&model.Provider{}).Where("id = ? AND version = ?", v.ID, expected).Updates(map[string]any{"name": v.Name, "provider_type": v.ProviderType, "endpoint": v.Endpoint, "capabilities": datatypes.JSON([]byte(v.CapabilitiesJSON)), "version": v.Version, "updated_at": v.UpdatedAt})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}
func (r *Repository) SetStatus(ctx context.Context, v *domain.Provider, expected int64) error {
	res := dal.FromContext(ctx, r.db).Model(&model.Provider{}).Where("id = ? AND version = ?", v.ID, expected).Updates(map[string]any{"status": string(v.Status), "version": v.Version, "updated_at": v.UpdatedAt})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}
func fromRow(row *model.Provider) domain.Provider {
	return domain.Provider{ID: row.ID, Name: row.Name, ProviderType: row.ProviderType, Endpoint: row.Endpoint, CapabilitiesJSON: string(row.Capabilities), Status: domain.Status(row.Status), Version: row.Version, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
