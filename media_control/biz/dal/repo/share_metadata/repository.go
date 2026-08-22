package share_metadata

import (
	"context"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	d "media_agent/media_control/biz/domain/share_metadata"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Create(ctx context.Context, v *d.Share) error {
	return dal.FromContext(ctx, r.db).Create(&model.ShareMetadata{ID: v.ID, WorkspaceID: v.WorkspaceID, ResourceType: v.ResourceType, ResourceID: v.ResourceID, OwnerUserID: v.OwnerUserID, Status: string(v.Status), CreatedAt: v.CreatedAt, RevokedAt: v.RevokedAt}).Error
}
func (r *Repository) ListOwned(ctx context.Context, o, w, rt, rid string) (out []d.Share, err error) {
	q := dal.FromContext(ctx, r.db).Where("owner_user_id=? AND workspace_id=?", o, w)
	if rt != "" {
		q = q.Where("resource_type=?", rt)
	}
	if rid != "" {
		q = q.Where("resource_id=?", rid)
	}
	var rows []model.ShareMetadata
	err = q.Order("created_at DESC").Find(&rows).Error
	for _, v := range rows {
		out = append(out, toShare(v))
	}
	return
}
func (r *Repository) GetOwned(ctx context.Context, id, o string) (*d.Share, error) {
	var v model.ShareMetadata
	if err := dal.FromContext(ctx, r.db).Where("id=? AND owner_user_id=?", id, o).First(&v).Error; err != nil {
		return nil, d.ErrNotFound
	}
	x := toShare(v)
	return &x, nil
}
func (r *Repository) Revoke(ctx context.Context, v *d.Share) error {
	res := dal.FromContext(ctx, r.db).Model(&model.ShareMetadata{}).Where("id=? AND owner_user_id=? AND status=?", v.ID, v.OwnerUserID, "active").Updates(map[string]any{"status": "revoked", "revoked_at": v.RevokedAt})
	if res.RowsAffected == 0 {
		return d.ErrRevoked
	}
	return res.Error
}
func toShare(v model.ShareMetadata) d.Share {
	return d.Share{ID: v.ID, WorkspaceID: v.WorkspaceID, ResourceType: v.ResourceType, ResourceID: v.ResourceID, OwnerUserID: v.OwnerUserID, Status: d.Status(v.Status), CreatedAt: v.CreatedAt, RevokedAt: v.RevokedAt}
}
