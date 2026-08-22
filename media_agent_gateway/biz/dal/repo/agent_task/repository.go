package agent_task

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
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
