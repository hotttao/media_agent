package credential_reference

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	domain "media_agent/media_control/biz/domain/credential_reference"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Create(ctx context.Context, v *domain.CredentialReference, secretRef string) error {
	ws := any(nil)
	if v.WorkspaceID != "" {
		ws = v.WorkspaceID
	}
	return dal.FromContext(ctx, r.db).Table("credential_references").Create(map[string]any{"id": v.ID, "workspace_id": ws, "owner_user_id": v.OwnerUserID, "name": v.Name, "provider_id": v.ProviderID, "secret_ref": secretRef, "status": string(v.Status), "version": v.Version, "created_at": v.CreatedAt, "updated_at": v.UpdatedAt}).Error
}
func (r *Repository) ListOwned(ctx context.Context, owner, workspaceID string) ([]domain.CredentialReference, error) {
	var rows []model.CredentialReference
	db := dal.FromContext(ctx, r.db).Where("owner_user_id = ? AND status = ?", owner, string(domain.StatusActive))
	if workspaceID != "" {
		db = db.Where("workspace_id = ?", workspaceID)
	}
	if err := db.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.CredentialReference, 0, len(rows))
	for i := range rows {
		out = append(out, fromRow(&rows[i]))
	}
	return out, nil
}
func (r *Repository) GetOwned(ctx context.Context, id, owner string) (*domain.CredentialReference, error) {
	var row model.CredentialReference
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
func (r *Repository) Update(ctx context.Context, v *domain.CredentialReference, secretRef string, expected int64) error {
	res := dal.FromContext(ctx, r.db).Model(&model.CredentialReference{}).Where("id = ? AND owner_user_id = ? AND status = ? AND version = ?", v.ID, v.OwnerUserID, string(domain.StatusActive), expected).Updates(map[string]any{"name": v.Name, "provider_id": v.ProviderID, "secret_ref": secretRef, "version": v.Version, "updated_at": v.UpdatedAt})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}
func (r *Repository) Revoke(ctx context.Context, v *domain.CredentialReference, expected int64) error {
	res := dal.FromContext(ctx, r.db).Model(&model.CredentialReference{}).Where("id = ? AND owner_user_id = ? AND status = ? AND version = ?", v.ID, v.OwnerUserID, string(domain.StatusActive), expected).Updates(map[string]any{"status": string(domain.StatusRevoked), "version": v.Version, "updated_at": v.UpdatedAt})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}
func fromRow(row *model.CredentialReference) domain.CredentialReference {
	ws := ""
	if row.WorkspaceID != nil {
		ws = *row.WorkspaceID
	}
	return domain.CredentialReference{ID: row.ID, WorkspaceID: ws, OwnerUserID: row.OwnerUserID, Name: row.Name, ProviderID: row.ProviderID, Status: domain.Status(row.Status), Version: row.Version, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
