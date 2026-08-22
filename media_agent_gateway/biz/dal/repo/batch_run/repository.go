package batch_run

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"media_agent/media_agent_gateway/biz/dal"
	"media_agent/media_agent_gateway/biz/dal/model"
	domain "media_agent/media_agent_gateway/biz/domain/batch_run"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, value *domain.BatchRun) error {
	db := dal.FromContext(ctx, r.db)
	if err := db.Create(toBatchRow(value)).Error; err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return domain.ErrConflict
		}
		return err
	}
	rows := make([]model.BatchRunItem, 0, len(value.Items))
	for _, item := range value.Items {
		rows = append(rows, toItemRow(value.ID, item))
	}
	return db.Create(&rows).Error
}

func (r *Repository) FindByKey(ctx context.Context, owner, workspace, key string) (*domain.BatchRun, error) {
	var row model.BatchRun
	err := dal.FromContext(ctx, r.db).Where("requested_by = ? AND workspace_id = ? AND idempotency_key = ?", owner, workspace, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.withItems(ctx, row)
}

func (r *Repository) ListOwned(ctx context.Context, owner, workspace string, status domain.Status) ([]domain.BatchRun, error) {
	var rows []model.BatchRun
	query := dal.FromContext(ctx, r.db).Where("requested_by = ?", owner)
	if workspace != "" {
		query = query.Where("workspace_id = ?", workspace)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.BatchRun, 0, len(rows))
	for _, row := range rows {
		value, err := r.withItems(ctx, row)
		if err != nil {
			return nil, err
		}
		result = append(result, *value)
	}
	return result, nil
}

func (r *Repository) GetOwned(ctx context.Context, id, owner string) (*domain.BatchRun, error) {
	var row model.BatchRun
	err := dal.FromContext(ctx, r.db).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.RequestedBy != owner {
		return nil, domain.ErrForbidden
	}
	return r.withItems(ctx, row)
}

func (r *Repository) CancelCreating(ctx context.Context, value *domain.BatchRun) error {
	db := dal.FromContext(ctx, r.db)
	result := db.Model(&model.BatchRun{}).Where("id = ? AND requested_by = ? AND status = ?", value.ID, value.RequestedBy, domain.StatusCreating).
		Updates(map[string]any{"status": domain.StatusCancelled, "cancelled_count": value.ItemCount, "finished_at": value.FinishedAt, "updated_at": value.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return db.Model(&model.BatchRunItem{}).Where("batch_run_id = ? AND status = ?", value.ID, domain.ItemPending).
		Updates(map[string]any{"status": domain.ItemCancelled, "updated_at": value.UpdatedAt}).Error
}

func (r *Repository) withItems(ctx context.Context, row model.BatchRun) (*domain.BatchRun, error) {
	var rows []model.BatchRunItem
	if err := dal.FromContext(ctx, r.db).Where("batch_run_id = ?", row.ID).Order("created_at ASC, item_key ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	value := toDomain(row)
	value.Items = make([]domain.Item, 0, len(rows))
	for _, item := range rows {
		value.Items = append(value.Items, domain.Item{ItemKey: item.ItemKey, SessionID: item.SessionID, AgentTaskID: pointerValue(item.AgentTaskID), InputJSON: item.Input, Status: domain.ItemStatus(item.Status), ErrorCode: pointerValue(item.ErrorCode), CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt})
	}
	return value, nil
}

func toBatchRow(value *domain.BatchRun) *model.BatchRun {
	return &model.BatchRun{ID: value.ID, WorkspaceID: value.WorkspaceID, RequestedBy: value.RequestedBy, IdempotencyKey: value.IdempotencyKey, Status: string(value.Status), ItemCount: value.ItemCount, NextCursor: value.NextCursor, CompletedCount: value.CompletedCount, FailedCount: value.FailedCount, CancelledCount: value.CancelledCount, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, FinishedAt: value.FinishedAt}
}
func toItemRow(batchID string, item domain.Item) model.BatchRunItem {
	return model.BatchRunItem{BatchRunID: batchID, ItemKey: item.ItemKey, SessionID: item.SessionID, AgentTaskID: nullable(item.AgentTaskID), Input: item.InputJSON, Status: string(item.Status), ErrorCode: nullable(item.ErrorCode), CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}
func toDomain(row model.BatchRun) *domain.BatchRun {
	return &domain.BatchRun{ID: row.ID, WorkspaceID: row.WorkspaceID, RequestedBy: row.RequestedBy, IdempotencyKey: row.IdempotencyKey, Status: domain.Status(row.Status), ItemCount: row.ItemCount, NextCursor: row.NextCursor, CompletedCount: row.CompletedCount, FailedCount: row.FailedCount, CancelledCount: row.CancelledCount, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, FinishedAt: row.FinishedAt}
}
func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
