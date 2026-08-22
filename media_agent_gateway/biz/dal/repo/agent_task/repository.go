package agent_task

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
	domain "media_agent/media_agent_gateway/biz/domain/agent_task"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Create(ctx context.Context, v *domain.Task) error {
	err := dal.FromContext(ctx, r.db).Create(toRow(v)).Error
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return domain.ErrConflict
	}
	return err
}
func (r *Repository) FindByKey(ctx context.Context, owner, source, sourceID, key string) (*domain.Task, error) {
	var row model.AgentTask
	err := dal.FromContext(ctx, r.db).Where("requested_by=? AND source=? AND source_resource_id=? AND idempotency_key=?", owner, source, sourceID, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toDomain(row), nil
}
func (r *Repository) ListOwned(ctx context.Context, owner, workspace string, status domain.Status) ([]domain.Task, error) {
	var rows []model.AgentTask
	q := dal.FromContext(ctx, r.db).Where("requested_by=?", owner)
	if workspace != "" {
		q = q.Where("workspace_id=?", workspace)
	}
	if status != "" {
		q = q.Where("status=?", status)
	}
	if err := q.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Task, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toDomain(row))
	}
	return out, nil
}
func (r *Repository) GetOwned(ctx context.Context, id, owner string) (*domain.Task, error) {
	var row model.AgentTask
	err := dal.FromContext(ctx, r.db).Where("id=?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.RequestedBy != owner {
		return nil, domain.ErrForbidden
	}
	return toDomain(row), nil
}
func (r *Repository) Cancel(ctx context.Context, v *domain.Task, from []domain.Status) error {
	values := make([]string, 0, len(from))
	for _, s := range from {
		values = append(values, string(s))
	}
	res := dal.FromContext(ctx, r.db).Model(&model.AgentTask{}).Where("id=? AND requested_by=? AND status IN ?", v.ID, v.RequestedBy, values).Updates(map[string]any{"status": v.Status, "finished_at": v.FinishedAt, "updated_at": v.UpdatedAt})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id string) (*domain.Task, error) {
	var row model.AgentTask
	err := dal.FromContext(ctx, r.db).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toDomain(row), nil
}

func (r *Repository) Claim(ctx context.Context, id string, now time.Time) (*domain.Task, error) {
	var row model.AgentTask
	res := dal.FromContext(ctx, r.db).Model(&row).Clauses(clause.Returning{}).
		Where("id = ? AND status IN ?", id, []string{string(domain.StatusQueued), string(domain.StatusRetryWait)}).
		Updates(map[string]any{"status": domain.StatusRunning, "attempt": gorm.Expr("attempt + 1"), "started_at": gorm.Expr("COALESCE(started_at, ?)", now), "updated_at": now})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected > 0 {
		return toDomain(row), nil
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	// A retry after an outcome-unknown worker attempt may resume the same running Task.
	if current.Status == domain.StatusRunning && current.TurnID == "" {
		return current, nil
	}
	return nil, domain.ErrConflict
}

func (r *Repository) MarkWaitingTurn(ctx context.Context, id, turnID string, now time.Time) error {
	res := dal.FromContext(ctx, r.db).Model(&model.AgentTask{}).
		Where("id = ? AND status = ? AND (turn_id IS NULL OR turn_id = ?)", id, domain.StatusRunning, turnID).
		Updates(map[string]any{"status": domain.StatusWaitingTurn, "turn_id": turnID, "error_code": nil, "error_message": nil, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	current, err := r.Get(ctx, id)
	if err == nil && current.Status == domain.StatusWaitingTurn && current.TurnID == turnID {
		return nil
	}
	if err == nil && current.Status == domain.StatusCancelled {
		attached := dal.FromContext(ctx, r.db).Model(&model.AgentTask{}).
			Where("id = ? AND status = ? AND (turn_id IS NULL OR turn_id = ?)", id, domain.StatusCancelled, turnID).
			Updates(map[string]any{"turn_id": turnID, "updated_at": now})
		if attached.Error != nil {
			return attached.Error
		}
		if attached.RowsAffected > 0 || current.TurnID == turnID {
			return domain.ErrCancelled
		}
	}
	return domain.ErrConflict
}

func (r *Repository) MarkRetryWait(ctx context.Context, id, code, message string, now time.Time) error {
	res := dal.FromContext(ctx, r.db).Model(&model.AgentTask{}).
		Where("id = ? AND status = ? AND turn_id IS NULL", id, domain.StatusRunning).
		Updates(map[string]any{"status": domain.StatusRetryWait, "error_code": nullable(code), "error_message": nullable(message), "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (r *Repository) MarkTerminal(ctx context.Context, id string, status domain.Status, resultRef, code, message string, now time.Time) error {
	if !domain.IsTerminal(status) {
		return domain.ErrConflict
	}
	res := dal.FromContext(ctx, r.db).Model(&model.AgentTask{}).
		Where("id = ? AND status IN ?", id, []string{string(domain.StatusRunning), string(domain.StatusWaitingTurn)}).
		Updates(map[string]any{"status": status, "result_ref": nullable(resultRef), "error_code": nullable(code), "error_message": nullable(message), "finished_at": now, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	current, err := r.Get(ctx, id)
	if err == nil && current.Status == status {
		return nil
	}
	return domain.ErrConflict
}

func (r *Repository) ListWaitingTurn(ctx context.Context, limit int) ([]domain.Task, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var rows []model.AgentTask
	if err := dal.FromContext(ctx, r.db).Where("status = ?", domain.StatusWaitingTurn).Order("updated_at ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Task, 0, len(rows))
	for _, row := range rows {
		result = append(result, *toDomain(row))
	}
	return result, nil
}

func (r *Repository) Enqueue(ctx context.Context, event *domain.OutboxEvent) error {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	return dal.FromContext(ctx, r.db).Clauses(clause.OnConflict{DoNothing: true}).Create(&model.AgentTaskOutbox{
		ID: event.ID, AggregateID: event.AggregateID, EventType: event.EventType, Payload: event.PayloadJSON,
		Attempts: event.Attempts, AvailableAt: event.AvailableAt, CreatedAt: event.CreatedAt, UpdatedAt: event.UpdatedAt,
	}).Error
}

func (r *Repository) ListPendingActions(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var rows []model.AgentTaskOutbox
	if err := dal.FromContext(ctx, r.db).Where("event_type IN ? AND delivered_at IS NULL AND available_at <= ?", []string{"agent_task.dispatch", "agent_task.cancel"}, time.Now().UTC()).Order("created_at ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.OutboxEvent, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.OutboxEvent{ID: row.ID, AggregateID: row.AggregateID, EventType: row.EventType, PayloadJSON: row.Payload, Attempts: row.Attempts, AvailableAt: row.AvailableAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
	}
	return result, nil
}

func (r *Repository) MarkDelivered(ctx context.Context, id string, now time.Time) error {
	return dal.FromContext(ctx, r.db).Model(&model.AgentTaskOutbox{}).Where("id = ? AND delivered_at IS NULL", id).Updates(map[string]any{"delivered_at": now, "updated_at": now}).Error
}

func (r *Repository) MarkDeliveryFailed(ctx context.Context, id, message string, now time.Time) error {
	return dal.FromContext(ctx, r.db).Model(&model.AgentTaskOutbox{}).Where("id = ? AND delivered_at IS NULL", id).Updates(map[string]any{"attempts": gorm.Expr("attempts + 1"), "last_error": message, "available_at": now.Add(time.Second), "updated_at": now}).Error
}
func toRow(v *domain.Task) *model.AgentTask {
	return &model.AgentTask{ID: v.ID, WorkspaceID: v.WorkspaceID, RequestedBy: v.RequestedBy, SessionID: nullable(v.SessionID), Source: v.Source, SourceResourceID: v.SourceResourceID, IdempotencyKey: v.IdempotencyKey, Input: v.InputJSON, Status: string(v.Status), Attempt: v.Attempt, TurnID: nullable(v.TurnID), ResultRef: nullable(v.ResultRefJSON), ErrorCode: nullable(v.ErrorCode), ErrorMessage: nullable(v.ErrorMessage), CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, StartedAt: v.StartedAt, FinishedAt: v.FinishedAt}
}
func toDomain(v model.AgentTask) *domain.Task {
	return &domain.Task{ID: v.ID, WorkspaceID: v.WorkspaceID, RequestedBy: v.RequestedBy, SessionID: value(v.SessionID), Source: v.Source, SourceResourceID: v.SourceResourceID, IdempotencyKey: v.IdempotencyKey, InputJSON: v.Input, Status: domain.Status(v.Status), Attempt: v.Attempt, TurnID: value(v.TurnID), ResultRefJSON: value(v.ResultRef), ErrorCode: value(v.ErrorCode), ErrorMessage: value(v.ErrorMessage), CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, StartedAt: v.StartedAt, FinishedAt: v.FinishedAt}
}
func nullable(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
