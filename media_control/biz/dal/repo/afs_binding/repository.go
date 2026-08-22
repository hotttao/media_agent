package afs_binding

import (
	"context"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	d "media_agent/media_control/biz/domain/afs_binding"
	"time"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Create(ctx context.Context, v *d.Binding) error {
	return dal.FromContext(ctx, r.db).Create(&model.AfsBinding{ID: v.ID, WorkspaceID: v.WorkspaceID, ExternalRef: v.ExternalRef, DesiredState: v.DesiredState, ObservedState: v.ObservedState, Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}).Error
}
func (r *Repository) ListOwned(ctx context.Context, o, w string) (out []d.Binding, err error) {
	var rows []model.AfsBinding
	err = dal.FromContext(ctx, r.db).Table("afs_bindings a").Joins("JOIN workspaces w ON w.id=a.workspace_id").Where("a.workspace_id=? AND w.owner_user_id=?", w, o).Order("a.updated_at DESC").Find(&rows).Error
	for _, v := range rows {
		out = append(out, toBinding(v))
	}
	return
}
func (r *Repository) GetOwned(ctx context.Context, id, o string) (*d.Binding, error) {
	var v model.AfsBinding
	if err := dal.FromContext(ctx, r.db).Table("afs_bindings a").Joins("JOIN workspaces w ON w.id=a.workspace_id").Where("a.id=? AND w.owner_user_id=?", id, o).First(&v).Error; err != nil {
		return nil, d.ErrNotFound
	}
	x := toBinding(v)
	return &x, nil
}
func (r *Repository) Delete(ctx context.Context, v *d.Binding) error {
	return dal.FromContext(ctx, r.db).Delete(&model.AfsBinding{}, "id=?", v.ID).Error
}
func (r *Repository) Reconcile(ctx context.Context, v *d.Binding, observed string) error {
	return dal.FromContext(ctx, r.db).Model(&model.AfsBinding{}).Where("id=? AND version=?", v.ID, v.Version-1).Updates(map[string]any{"observed_state": observed, "version": v.Version, "updated_at": v.UpdatedAt}).Error
}
func (r *Repository) ListMembers(ctx context.Context, id string) (out []d.Member, err error) {
	var rows []model.AfsBindingMember
	err = dal.FromContext(ctx, r.db).Where("binding_id=?", id).Order("subject_id").Find(&rows).Error
	for _, v := range rows {
		out = append(out, toMember(v))
	}
	return
}
func (r *Repository) AddMember(ctx context.Context, v *d.Member) error {
	return dal.FromContext(ctx, r.db).Create(&model.AfsBindingMember{BindingID: v.BindingID, SubjectID: v.SubjectID, SubjectType: v.SubjectType, ExternalMemberRef: v.ExternalMemberRef, Status: v.Status, UpdatedAt: v.UpdatedAt}).Error
}
func (r *Repository) RemoveMember(ctx context.Context, id, subject string) error {
	return dal.FromContext(ctx, r.db).Model(&model.AfsBindingMember{}).Where("binding_id=? AND subject_id=?", id, subject).Updates(map[string]any{"status": "removed", "updated_at": time.Now().UTC()}).Error
}
func toBinding(v model.AfsBinding) d.Binding {
	return d.Binding{ID: v.ID, WorkspaceID: v.WorkspaceID, ExternalRef: v.ExternalRef, DesiredState: v.DesiredState, ObservedState: v.ObservedState, Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func toMember(v model.AfsBindingMember) d.Member {
	return d.Member{BindingID: v.BindingID, SubjectID: v.SubjectID, SubjectType: v.SubjectType, ExternalMemberRef: v.ExternalMemberRef, Status: v.Status, UpdatedAt: v.UpdatedAt}
}
