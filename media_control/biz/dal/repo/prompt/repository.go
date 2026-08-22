package prompt

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	domain "media_agent/media_control/biz/domain/prompt"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, value *domain.Prompt) error {
	workspaceID := any(nil)
	if value.WorkspaceID != "" {
		workspaceID = value.WorkspaceID
	}
	return dal.FromContext(ctx, r.db).Table("prompts").Create(map[string]any{
		"id": value.ID, "workspace_id": workspaceID, "owner_user_id": value.OwnerUserID,
		"name": value.Name, "content": value.Content, "status": string(value.Status),
		"version": value.Version, "created_at": value.CreatedAt, "updated_at": value.UpdatedAt,
	}).Error
}

func (r *Repository) ListOwned(ctx context.Context, owner, workspaceID string) ([]domain.Prompt, error) {
	var rows []model.Prompt
	db := dal.FromContext(ctx, r.db).Where("owner_user_id = ? AND status = ?", owner, string(domain.StatusActive))
	if workspaceID != "" {
		db = db.Where("workspace_id = ?", workspaceID)
	}
	if err := db.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Prompt, 0, len(rows))
	for i := range rows {
		result = append(result, fromRow(&rows[i]))
	}
	return result, nil
}

func (r *Repository) GetOwned(ctx context.Context, id, owner string) (*domain.Prompt, error) {
	var row model.Prompt
	err := dal.FromContext(ctx, r.db).Where("id = ? AND owner_user_id = ?", id, owner).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	value := fromRow(&row)
	return &value, nil
}

func (r *Repository) Update(ctx context.Context, value *domain.Prompt, expected int64) error {
	result := dal.FromContext(ctx, r.db).Model(&model.Prompt{}).
		Where("id = ? AND owner_user_id = ? AND status = ? AND version = ?", value.ID, value.OwnerUserID, string(domain.StatusActive), expected).
		Updates(map[string]any{"name": value.Name, "content": value.Content, "version": value.Version, "updated_at": value.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (r *Repository) Archive(ctx context.Context, value *domain.Prompt, expected int64) error {
	result := dal.FromContext(ctx, r.db).Model(&model.Prompt{}).
		Where("id = ? AND owner_user_id = ? AND status = ? AND version = ?", value.ID, value.OwnerUserID, string(domain.StatusActive), expected).
		Updates(map[string]any{"status": string(domain.StatusArchived), "version": value.Version, "updated_at": value.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}

func toRow(value *domain.Prompt) *model.Prompt {
	var workspaceID *string
	if value.WorkspaceID != "" {
		workspaceID = &value.WorkspaceID
	}
	return &model.Prompt{ID: value.ID, WorkspaceID: workspaceID, OwnerUserID: value.OwnerUserID, Name: value.Name, Content: value.Content, Status: string(value.Status), Version: value.Version, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}

func fromRow(row *model.Prompt) domain.Prompt {
	workspaceID := ""
	if row.WorkspaceID != nil {
		workspaceID = *row.WorkspaceID
	}
	return domain.Prompt{ID: row.ID, WorkspaceID: workspaceID, OwnerUserID: row.OwnerUserID, Name: row.Name, Content: row.Content, Status: domain.Status(row.Status), Version: row.Version, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
