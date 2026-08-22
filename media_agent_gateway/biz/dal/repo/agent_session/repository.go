package agent_session

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"media_agent/media_agent_gateway/biz/dal/model"
	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) FindLatestUsable(ctx context.Context, userID string) (*domain.Session, error) {
	var row model.AgentSession
	err := r.db.WithContext(ctx).Where("user_id = ? AND status IN ?", userID, []string{domain.StatusStarting, domain.StatusActive}).Order("updated_at DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toSession(row), nil
}

func (r *Repository) TransitionStatus(ctx context.Context, sessionID, from, to string, updatedAt time.Time) error {
	result := r.db.WithContext(ctx).Model(&model.AgentSession{}).
		Where("id = ? AND status = ?", sessionID, from).
		Updates(map[string]any{"status": to, "updated_at": updatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repository) CreateSession(ctx context.Context, session *domain.Session) error {
	return r.db.WithContext(ctx).Create(&model.AgentSession{
		ID: session.ID, UserID: session.UserID, WorkspaceID: session.WorkspaceID,
		AgentID: session.AgentID, AgentType: session.AgentType, Title: session.Title,
		ContextVersion: session.ContextVersion, Status: session.Status,
		CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt,
	}).Error
}

func (r *Repository) ListOwned(ctx context.Context, userID, workspaceID string) ([]domain.Session, error) {
	var rows []model.AgentSession
	db := r.db.WithContext(ctx).Where("user_id = ? AND status <> ?", userID, "deleted")
	if workspaceID != "" {
		db = db.Where("workspace_id = ?", workspaceID)
	}
	if err := db.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Session, 0, len(rows))
	for _, row := range rows {
		result = append(result, *toSession(row))
	}
	return result, nil
}

func (r *Repository) GetOwned(ctx context.Context, sessionID, userID string) (*domain.Session, error) {
	var row model.AgentSession
	err := r.db.WithContext(ctx).Where("id = ?", sessionID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.UserID != userID {
		return nil, domain.ErrForbidden
	}
	return toSession(row), nil
}

func toSession(row model.AgentSession) *domain.Session {
	return &domain.Session{
		ID: row.ID, UserID: row.UserID, WorkspaceID: row.WorkspaceID,
		AgentID: row.AgentID, AgentType: row.AgentType, Title: row.Title,
		ContextVersion: row.ContextVersion, Status: row.Status,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
