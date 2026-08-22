package memory_store

import (
	"context"
	"errors"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	domain "media_agent/media_control/biz/domain/memory_store"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Create(ctx context.Context, v *domain.MemoryStore) error {
	return dal.FromContext(ctx, r.db).Table("memory_stores").Create(map[string]any{"id": v.ID, "workspace_id": v.WorkspaceID, "owner_user_id": v.OwnerUserID, "name": v.Name, "store_type": v.StoreType, "configuration": datatypes.JSON([]byte(v.ConfigurationJSON)), "credential_reference_id": nullable(v.CredentialReferenceID), "status": string(v.Status), "version": v.Version, "created_at": v.CreatedAt, "updated_at": v.UpdatedAt}).Error
}
func (r *Repository) ListOwned(ctx context.Context, owner, workspaceID string) ([]domain.MemoryStore, error) {
	var rows []model.MemoryStore
	db := dal.FromContext(ctx, r.db).Where("owner_user_id = ? AND workspace_id = ?", owner, workspaceID)
	if err := db.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.MemoryStore, 0, len(rows))
	for i := range rows {
		out = append(out, fromRow(&rows[i]))
	}
	return out, nil
}
func (r *Repository) GetOwned(ctx context.Context, id, owner string) (*domain.MemoryStore, error) {
	var row model.MemoryStore
	err := dal.FromContext(ctx, r.db).Where("id = ? AND owner_user_id = ?", id, owner).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v := fromRow(&row)
	return &v, nil
}
func (r *Repository) Update(ctx context.Context, v *domain.MemoryStore, expected int64) error {
	res := dal.FromContext(ctx, r.db).Model(&model.MemoryStore{}).Where("id = ? AND owner_user_id = ? AND version = ?", v.ID, v.OwnerUserID, expected).Updates(map[string]any{"name": v.Name, "store_type": v.StoreType, "configuration": datatypes.JSON([]byte(v.ConfigurationJSON)), "credential_reference_id": nullable(v.CredentialReferenceID), "version": v.Version, "updated_at": v.UpdatedAt})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}
func (r *Repository) SetStatus(ctx context.Context, v *domain.MemoryStore, expected int64) error {
	res := dal.FromContext(ctx, r.db).Model(&model.MemoryStore{}).Where("id = ? AND owner_user_id = ? AND version = ?", v.ID, v.OwnerUserID, expected).Updates(map[string]any{"status": string(v.Status), "version": v.Version, "updated_at": v.UpdatedAt})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}
func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func fromRow(row *model.MemoryStore) domain.MemoryStore {
	cred := ""
	if row.CredentialReferenceID != nil {
		cred = *row.CredentialReferenceID
	}
	return domain.MemoryStore{ID: row.ID, WorkspaceID: row.WorkspaceID, OwnerUserID: row.OwnerUserID, Name: row.Name, StoreType: row.StoreType, ConfigurationJSON: string(row.Configuration), CredentialReferenceID: cred, Status: domain.Status(row.Status), Version: row.Version, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
