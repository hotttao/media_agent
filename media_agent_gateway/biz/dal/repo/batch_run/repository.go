package batch_run

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

func (r *Repository) Get(ctx context.Context, id string) (*domain.BatchRun, error) {
	var row model.BatchRun
	err := dal.FromContext(ctx, r.db).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.withItems(ctx, row)
}

func (r *Repository) AttachTask(ctx context.Context, batchID string, expectedCursor int32, itemKey, taskID string, now time.Time) error {
	db := dal.FromContext(ctx, r.db)
	result := db.Model(&model.BatchRun{}).
		Where("id = ? AND status IN ? AND next_cursor = ?", batchID, []string{string(domain.StatusCreating), string(domain.StatusRunning)}, expectedCursor).
		Updates(map[string]any{"status": domain.StatusRunning, "next_cursor": expectedCursor + 1, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		current, err := r.Get(ctx, batchID)
		if err == nil && current.NextCursor > expectedCursor {
			for _, item := range current.Items {
				if item.ItemKey == itemKey && item.AgentTaskID == taskID {
					return nil
				}
			}
		}
		return domain.ErrConflict
	}
	itemResult := db.Model(&model.BatchRunItem{}).
		Where("batch_run_id = ? AND item_key = ? AND status = ? AND agent_task_id IS NULL", batchID, itemKey, domain.ItemPending).
		Updates(map[string]any{"agent_task_id": taskID, "status": domain.ItemQueued, "updated_at": now})
	if itemResult.Error != nil {
		return itemResult.Error
	}
	if itemResult.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (r *Repository) Cancel(ctx context.Context, value *domain.BatchRun) error {
	db := dal.FromContext(ctx, r.db)
	items := db.Model(&model.BatchRunItem{}).
		Where("batch_run_id = ? AND status = ? AND agent_task_id IS NULL", value.ID, domain.ItemPending).
		Updates(map[string]any{"status": domain.ItemCancelled, "updated_at": value.UpdatedAt})
	if items.Error != nil {
		return items.Error
	}
	result := db.Model(&model.BatchRun{}).Where("id = ? AND requested_by = ? AND status IN ?", value.ID, value.RequestedBy, []string{string(domain.StatusCreating), string(domain.StatusRunning)}).
		Updates(map[string]any{"status": domain.StatusCancelled, "cancelled_count": gorm.Expr("cancelled_count + ?", items.RowsAffected), "finished_at": value.FinishedAt, "updated_at": value.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (r *Repository) ApplyTaskTerminal(ctx context.Context, event domain.TaskTerminal, now time.Time) error {
	db := dal.FromContext(ctx, r.db)
	var item model.BatchRunItem
	err := db.Where("agent_task_id = ?", event.TaskID).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	inbox := &model.BatchRunInbox{EventID: event.EventID, BatchRunID: item.BatchRunID, ItemKey: item.ItemKey, CreatedAt: now}
	insert := db.Clauses(clause.OnConflict{DoNothing: true}).Create(inbox)
	if insert.Error != nil {
		return insert.Error
	}
	if insert.RowsAffected == 0 {
		return nil
	}
	itemStatus := domain.ItemFailed
	switch event.Status {
	case "completed":
		itemStatus = domain.ItemCompleted
	case "cancelled":
		itemStatus = domain.ItemCancelled
	}
	if err := db.Model(&model.BatchRunItem{}).Where("batch_run_id = ? AND item_key = ? AND status IN ?", item.BatchRunID, item.ItemKey, []string{string(domain.ItemQueued), string(domain.ItemRunning)}).
		Updates(map[string]any{"status": itemStatus, "error_code": nullable(event.ErrorCode), "updated_at": now}).Error; err != nil {
		return err
	}
	var counts struct{ Completed, Failed, Cancelled int32 }
	if err := db.Model(&model.BatchRunItem{}).Select(
		"COUNT(*) FILTER (WHERE status = 'completed') AS completed, COUNT(*) FILTER (WHERE status = 'failed') AS failed, COUNT(*) FILTER (WHERE status = 'cancelled') AS cancelled",
	).Where("batch_run_id = ?", item.BatchRunID).Scan(&counts).Error; err != nil {
		return err
	}
	var batch model.BatchRun
	if err := db.Where("id = ?", item.BatchRunID).First(&batch).Error; err != nil {
		return err
	}
	updates := map[string]any{"completed_count": counts.Completed, "failed_count": counts.Failed, "cancelled_count": counts.Cancelled, "updated_at": now}
	terminal := counts.Completed + counts.Failed + counts.Cancelled
	if terminal == batch.ItemCount && batch.Status != string(domain.StatusCancelled) {
		status := domain.StatusCompleted
		if counts.Failed == batch.ItemCount {
			status = domain.StatusFailed
		} else if counts.Failed > 0 || counts.Cancelled > 0 {
			status = domain.StatusPartialFailed
		}
		updates["status"], updates["finished_at"] = status, now
	}
	return db.Model(&model.BatchRun{}).Where("id = ?", item.BatchRunID).Updates(updates).Error
}

func (r *Repository) Enqueue(ctx context.Context, event *domain.OutboxEvent) error {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	return dal.FromContext(ctx, r.db).Clauses(clause.OnConflict{DoNothing: true}).Create(&model.BatchRunOutbox{ID: event.ID, BatchRunID: event.BatchRunID, Action: event.Action, Payload: event.PayloadJSON, Attempts: event.Attempts, AvailableAt: event.AvailableAt, CreatedAt: event.CreatedAt, UpdatedAt: event.UpdatedAt}).Error
}

func (r *Repository) ListPendingActions(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var rows []model.BatchRunOutbox
	if err := dal.FromContext(ctx, r.db).Where("delivered_at IS NULL AND available_at <= ?", time.Now().UTC()).Order("created_at ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.OutboxEvent, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.OutboxEvent{ID: row.ID, BatchRunID: row.BatchRunID, Action: row.Action, PayloadJSON: row.Payload, Attempts: row.Attempts, AvailableAt: row.AvailableAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
	}
	return result, nil
}
func (r *Repository) MarkDelivered(ctx context.Context, id string, now time.Time) error {
	return dal.FromContext(ctx, r.db).Model(&model.BatchRunOutbox{}).Where("id = ? AND delivered_at IS NULL", id).Updates(map[string]any{"delivered_at": now, "updated_at": now}).Error
}
func (r *Repository) MarkDeliveryFailed(ctx context.Context, id, message string, now time.Time) error {
	return dal.FromContext(ctx, r.db).Model(&model.BatchRunOutbox{}).Where("id = ? AND delivered_at IS NULL", id).Updates(map[string]any{"attempts": gorm.Expr("attempts + 1"), "last_error": message, "available_at": now.Add(time.Second), "updated_at": now}).Error
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
