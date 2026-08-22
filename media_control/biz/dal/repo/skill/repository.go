package skill

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	domain "media_agent/media_control/biz/domain/skill"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, value *domain.Skill) error {
	workspaceID := any(nil)
	if value.WorkspaceID != "" {
		workspaceID = value.WorkspaceID
	}
	return dal.FromContext(ctx, r.db).Table("skills").Create(map[string]any{
		"id": value.ID, "workspace_id": workspaceID, "owner_user_id": value.OwnerUserID, "name": value.Name,
		"content_path": value.ContentPath, "checksum": value.Checksum, "status": string(value.Status), "version": value.Version,
		"created_at": value.CreatedAt, "updated_at": value.UpdatedAt,
	}).Error
}

func (r *Repository) ListOwned(ctx context.Context, owner, workspaceID string) ([]domain.Skill, error) {
	var rows []model.Skill
	db := dal.FromContext(ctx, r.db).Where("owner_user_id = ? AND status <> ?", owner, string(domain.StatusArchived))
	if workspaceID != "" {
		db = db.Where("workspace_id = ?", workspaceID)
	}
	if err := db.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Skill, 0, len(rows))
	for i := range rows {
		result = append(result, fromRow(&rows[i]))
	}
	return result, nil
}

func (r *Repository) GetOwned(ctx context.Context, id, owner string) (*domain.Skill, error) {
	var row model.Skill
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

func (r *Repository) Update(ctx context.Context, value *domain.Skill, expected int64) error {
	result := dal.FromContext(ctx, r.db).Model(&model.Skill{}).Where("id = ? AND owner_user_id = ? AND status <> ? AND version = ?", value.ID, value.OwnerUserID, string(domain.StatusArchived), expected).Updates(map[string]any{"name": value.Name, "content_path": value.ContentPath, "checksum": value.Checksum, "version": value.Version, "updated_at": value.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (r *Repository) SetStatus(ctx context.Context, value *domain.Skill, expected int64) error {
	result := dal.FromContext(ctx, r.db).Model(&model.Skill{}).Where("id = ? AND owner_user_id = ? AND status <> ? AND version = ?", value.ID, value.OwnerUserID, string(domain.StatusArchived), expected).Updates(map[string]any{"status": string(value.Status), "version": value.Version, "updated_at": value.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (r *Repository) Archive(ctx context.Context, value *domain.Skill, expected int64) error {
	result := dal.FromContext(ctx, r.db).Model(&model.Skill{}).Where("id = ? AND owner_user_id = ? AND status <> ? AND version = ?", value.ID, value.OwnerUserID, string(domain.StatusArchived), expected).Updates(map[string]any{"status": string(domain.StatusArchived), "version": value.Version, "updated_at": value.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}

func fromRow(row *model.Skill) domain.Skill {
	workspaceID := ""
	if row.WorkspaceID != nil {
		workspaceID = *row.WorkspaceID
	}
	return domain.Skill{ID: row.ID, WorkspaceID: workspaceID, OwnerUserID: row.OwnerUserID, Name: row.Name, ContentPath: row.ContentPath, Checksum: row.Checksum, Status: domain.Status(row.Status), Version: row.Version, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
