package workspace_metadata

import (
	"context"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	d "media_agent/media_control/biz/domain/workspace_metadata"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Create(ctx context.Context, v *d.Tag) error {
	return dal.FromContext(ctx, r.db).Create(&model.Tag{ID: v.ID, WorkspaceID: v.WorkspaceID, Name: v.Name, Color: v.Color, CreatedBy: v.CreatedBy, Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}).Error
}
func (r *Repository) ListOwned(ctx context.Context, o, w string) (out []d.Tag, err error) {
	var rows []model.Tag
	err = dal.FromContext(ctx, r.db).Table("tags t").Joins("JOIN workspaces w ON w.id=t.workspace_id").Where("t.workspace_id=? AND w.owner_user_id=?", w, o).Order("t.name ASC").Find(&rows).Error
	for _, v := range rows {
		out = append(out, toTag(v))
	}
	return
}
func (r *Repository) GetOwned(ctx context.Context, id, o string) (*d.Tag, error) {
	var v model.Tag
	if err := dal.FromContext(ctx, r.db).Table("tags t").Joins("JOIN workspaces w ON w.id=t.workspace_id").Where("t.id=? AND w.owner_user_id=?", id, o).First(&v).Error; err != nil {
		return nil, d.ErrNotFound
	}
	x := toTag(v)
	return &x, nil
}
func (r *Repository) Update(ctx context.Context, v *d.Tag, ev int64) error {
	res := dal.FromContext(ctx, r.db).Model(&model.Tag{}).Where("id=? AND version=?", v.ID, ev).Updates(map[string]any{"name": v.Name, "color": v.Color, "version": v.Version, "updated_at": v.UpdatedAt})
	if res.RowsAffected == 0 {
		return d.ErrConflict
	}
	return res.Error
}
func (r *Repository) Delete(ctx context.Context, v *d.Tag, ev int64) error {
	res := dal.FromContext(ctx, r.db).Where("id=? AND version=?", v.ID, ev).Delete(&model.Tag{})
	if res.RowsAffected == 0 {
		return d.ErrConflict
	}
	return res.Error
}
func (r *Repository) Attach(ctx context.Context, v *d.ResourceTag) error {
	return dal.FromContext(ctx, r.db).Create(&model.ResourceTag{TagID: v.TagID, WorkspaceID: v.WorkspaceID, ResourceType: v.ResourceType, ResourceID: v.ResourceID, CreatedAt: v.CreatedAt}).Error
}
func (r *Repository) Detach(ctx context.Context, v *d.ResourceTag) error {
	res := dal.FromContext(ctx, r.db).Where("tag_id=? AND workspace_id=? AND resource_type=? AND resource_id=?", v.TagID, v.WorkspaceID, v.ResourceType, v.ResourceID).Delete(&model.ResourceTag{})
	if res.RowsAffected == 0 {
		return d.ErrAssociationNotFound
	}
	return res.Error
}
func (r *Repository) ListResourceTags(ctx context.Context, o, w, rt, rid string) (out []d.Tag, err error) {
	var rows []model.Tag
	err = dal.FromContext(ctx, r.db).Table("tags t").Joins("JOIN resource_tags rt ON rt.tag_id=t.id").Joins("JOIN workspaces w ON w.id=t.workspace_id").Where("rt.workspace_id=? AND rt.resource_type=? AND rt.resource_id=? AND w.owner_user_id=?", w, rt, rid, o).Order("t.name ASC").Find(&rows).Error
	for _, v := range rows {
		out = append(out, toTag(v))
	}
	return
}
func toTag(v model.Tag) d.Tag {
	return d.Tag{ID: v.ID, WorkspaceID: v.WorkspaceID, Name: v.Name, Color: v.Color, CreatedBy: v.CreatedBy, Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
