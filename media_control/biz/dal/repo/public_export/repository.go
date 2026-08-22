package public_export

import (
	"context"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	d "media_agent/media_control/biz/domain/public_export"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Create(ctx context.Context, v *d.Export) error {
	return dal.FromContext(ctx, r.db).Create(&model.PublicExport{ID: v.ID, WorkspaceID: v.WorkspaceID, ResourceType: v.ResourceType, ResourceID: v.ResourceID, ObjectPath: v.ObjectPath, Status: string(v.Status), CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt}).Error
}
func (r *Repository) ListOwned(ctx context.Context, o, w, rt, rid string) (out []d.Export, err error) {
	q := dal.FromContext(ctx, r.db).Where("created_by=? AND workspace_id=?", o, w)
	if rt != "" {
		q = q.Where("resource_type=?", rt)
	}
	if rid != "" {
		q = q.Where("resource_id=?", rid)
	}
	var rows []model.PublicExport
	err = q.Order("created_at DESC").Find(&rows).Error
	for _, v := range rows {
		out = append(out, toExport(v))
	}
	return
}
func (r *Repository) GetOwned(ctx context.Context, id, o string) (*d.Export, error) {
	var v model.PublicExport
	if err := dal.FromContext(ctx, r.db).Where("id=? AND created_by=?", id, o).First(&v).Error; err != nil {
		return nil, d.ErrNotFound
	}
	x := toExport(v)
	return &x, nil
}
func (r *Repository) SetStatus(ctx context.Context, v *d.Export, from string) error {
	res := dal.FromContext(ctx, r.db).Model(&model.PublicExport{}).Where("id=? AND created_by=? AND status=?", v.ID, v.CreatedBy, from).Update("status", v.Status)
	if res.RowsAffected == 0 {
		return d.ErrTransition
	}
	return res.Error
}
func toExport(v model.PublicExport) d.Export {
	return d.Export{ID: v.ID, WorkspaceID: v.WorkspaceID, ResourceType: v.ResourceType, ResourceID: v.ResourceID, ObjectPath: v.ObjectPath, Status: d.Status(v.Status), CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt}
}
