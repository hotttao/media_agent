package runtime_environment

import (
	"context"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	d "media_agent/media_control/biz/domain/runtime_environment"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) CreateEnvironment(ctx context.Context, v *d.Environment) error {
	return dal.FromContext(ctx, r.db).Create(&model.RuntimeEnvironment{ID: v.ID, Name: v.Name, Endpoint: v.Endpoint, Capacity: datatypesJSON(v.CapacityJSON), Status: string(v.Status), Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}).Error
}
func (r *Repository) ListEnvironments(ctx context.Context) ([]d.Environment, error) {
	var rows []model.RuntimeEnvironment
	if err := dal.FromContext(ctx, r.db).Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]d.Environment, 0, len(rows))
	for _, v := range rows {
		out = append(out, toEnvironment(v))
	}
	return out, nil
}
func (r *Repository) GetEnvironment(ctx context.Context, id string) (*d.Environment, error) {
	var v model.RuntimeEnvironment
	if err := dal.FromContext(ctx, r.db).Where("id=?", id).First(&v).Error; err != nil {
		return nil, d.ErrNotFound
	}
	x := toEnvironment(v)
	return &x, nil
}
func (r *Repository) UpdateEnvironment(ctx context.Context, v *d.Environment, expected int64) error {
	res := dal.FromContext(ctx, r.db).Model(&model.RuntimeEnvironment{}).Where("id=? AND version=?", v.ID, expected).Updates(map[string]any{"name": v.Name, "endpoint": v.Endpoint, "capacity": datatypesJSON(v.CapacityJSON), "status": v.Status, "version": v.Version, "updated_at": v.UpdatedAt})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return d.ErrConflict
	}
	return nil
}
func (r *Repository) DeleteEnvironment(ctx context.Context, v *d.Environment, expected int64) error {
	return r.UpdateEnvironment(ctx, v, expected)
}
func (r *Repository) FindPlacement(ctx context.Context, wid string) (*d.Placement, error) {
	var v model.WorkspacePlacement
	if err := dal.FromContext(ctx, r.db).Where("workspace_id=?", wid).First(&v).Error; err != nil {
		return nil, d.ErrPlacementNotFound
	}
	return toPlacement(v), nil
}
func (r *Repository) UpsertPlacement(ctx context.Context, v *d.Placement, expected int64) error {
	db := dal.FromContext(ctx, r.db)
	var current model.WorkspacePlacement
	err := db.Where("workspace_id=?", v.WorkspaceID).First(&current).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if err == gorm.ErrRecordNotFound {
		return db.Create(&model.WorkspacePlacement{WorkspaceID: v.WorkspaceID, EnvironmentID: nullable(v.EnvironmentID), Status: string(v.Status), Reason: v.Reason, VolumeRef: v.VolumeRef, Version: v.Version, UpdatedAt: v.UpdatedAt}).Error
	}
	if current.Version != expected {
		return d.ErrConflict
	}
	res := db.Model(&model.WorkspacePlacement{}).Where("workspace_id=? AND version=?", v.WorkspaceID, expected).Updates(map[string]any{"environment_id": nullable(v.EnvironmentID), "status": v.Status, "reason": v.Reason, "volume_ref": v.VolumeRef, "version": v.Version, "updated_at": v.UpdatedAt})
	if res.RowsAffected == 0 {
		return d.ErrConflict
	}
	return res.Error
}
func nullable(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func datatypesJSON(v string) []byte { return []byte(v) }
func toEnvironment(v model.RuntimeEnvironment) d.Environment {
	return d.Environment{ID: v.ID, Name: v.Name, Endpoint: v.Endpoint, CapacityJSON: string(v.Capacity), Status: d.EnvironmentStatus(v.Status), Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func toPlacement(v model.WorkspacePlacement) *d.Placement {
	e := ""
	if v.EnvironmentID != nil {
		e = *v.EnvironmentID
	}
	return &d.Placement{WorkspaceID: v.WorkspaceID, EnvironmentID: e, Status: d.PlacementStatus(v.Status), Reason: v.Reason, VolumeRef: v.VolumeRef, Version: v.Version, UpdatedAt: v.UpdatedAt}
}
