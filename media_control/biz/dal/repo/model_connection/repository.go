package model_connection

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	d "media_agent/media_control/biz/domain/model_connection"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db} }
func (r *Repository) Get(ctx context.Context, id string) (*d.Connection, error) {
	var v d.Connection
	e := dal.FromContext(ctx, r.db).Table("model_connections").Where("id=?", id).Take(&v).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = d.ErrNotFound
	}
	return &v, e
}
func (r *Repository) List(ctx context.Context, owner string) ([]d.Connection, error) {
	v := []d.Connection{}
	e := dal.FromContext(ctx, r.db).Table("model_connections").Select("id,owner_user_id,name,kind,status,version,created_at,updated_at").Where("owner_user_id=?", owner).Order("created_at DESC").Find(&v).Error
	return v, e
}
func (r *Repository) Save(ctx context.Context, v *d.Connection, expected int64) error {
	db := dal.FromContext(ctx, r.db).Table("model_connections")
	if expected == 0 {
		return db.Create(v).Error
	}
	result := db.Where("id=? AND owner_user_id=? AND version=?", v.ID, v.OwnerUserID, expected).Updates(map[string]any{"name": v.Name, "status": v.Status, "version": v.Version, "encrypted_auth": v.EncryptedAuth, "updated_at": v.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return d.ErrConflict
	}
	return nil
}
