package schedule

import (
	"context"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	d "media_agent/media_control/biz/domain/schedule"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Create(ctx context.Context, v *d.Schedule) error {
	return dal.FromContext(ctx, r.db).Create(&model.AgentSchedule{ID: v.ID, WorkspaceID: v.WorkspaceID, RequestedBy: v.RequestedBy, AgentConfigID: v.AgentConfigID, Rule: v.Rule, Timezone: v.Timezone, Status: string(v.Status), Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}).Error
}
func (r *Repository) ListOwned(ctx context.Context, o, w string) (out []d.Schedule, err error) {
	var rows []model.AgentSchedule
	err = dal.FromContext(ctx, r.db).Where("requested_by=? AND workspace_id=? AND status <> ?", o, w, "deleted").Order("updated_at DESC").Find(&rows).Error
	for _, v := range rows {
		out = append(out, toDomain(v))
	}
	return
}
func (r *Repository) GetOwned(ctx context.Context, id, o string) (*d.Schedule, error) {
	var v model.AgentSchedule
	if err := dal.FromContext(ctx, r.db).Where("id=? AND requested_by=?", id, o).First(&v).Error; err != nil {
		return nil, d.ErrNotFound
	}
	x := toDomain(v)
	return &x, nil
}
func (r *Repository) Update(ctx context.Context, v *d.Schedule, ev int64) error {
	res := dal.FromContext(ctx, r.db).Model(&model.AgentSchedule{}).Where("id=? AND requested_by=? AND version=?", v.ID, v.RequestedBy, ev).Updates(map[string]any{"rule": v.Rule, "timezone": v.Timezone, "status": v.Status, "version": v.Version, "updated_at": v.UpdatedAt})
	if res.RowsAffected == 0 {
		return d.ErrConflict
	}
	return res.Error
}
func (r *Repository) Delete(ctx context.Context, v *d.Schedule, ev int64) error {
	return r.Update(ctx, v, ev)
}
func (r *Repository) CreateTrigger(ctx context.Context, v *d.Trigger) error {
	return dal.FromContext(ctx, r.db).Create(&model.ScheduleTrigger{ID: v.ID, ScheduleID: v.ScheduleID, DueAt: v.DueAt, Status: v.Status, AgentTaskID: v.AgentTaskID, ErrorCode: v.ErrorCode, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}).Error
}
func (r *Repository) GetTrigger(ctx context.Context, id string) (*d.Trigger, error) {
	var v model.ScheduleTrigger
	if err := dal.FromContext(ctx, r.db).Where("id=?", id).First(&v).Error; err != nil {
		return nil, d.ErrNotFound
	}
	return &d.Trigger{ID: v.ID, ScheduleID: v.ScheduleID, Status: v.Status, AgentTaskID: v.AgentTaskID, ErrorCode: v.ErrorCode, DueAt: v.DueAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}, nil
}
func (r *Repository) UpdateTrigger(ctx context.Context, v *d.Trigger, from string) error {
	res := dal.FromContext(ctx, r.db).Model(&model.ScheduleTrigger{}).Where("id=? AND status=?", v.ID, from).Updates(map[string]any{"status": v.Status, "agent_task_id": v.AgentTaskID, "error_code": v.ErrorCode, "updated_at": v.UpdatedAt})
	if res.RowsAffected == 0 {
		return d.ErrConflict
	}
	return res.Error
}
func toDomain(v model.AgentSchedule) d.Schedule {
	return d.Schedule{ID: v.ID, WorkspaceID: v.WorkspaceID, RequestedBy: v.RequestedBy, AgentConfigID: v.AgentConfigID, Rule: v.Rule, Timezone: v.Timezone, Status: d.Status(v.Status), NextRunAt: v.NextRunAt, Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
