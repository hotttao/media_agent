package plugin_binding

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"media_agent/media_plugin/biz/dal"
	d "media_agent/media_plugin/biz/domain/plugin_binding"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db} }

type row struct {
	ID, WorkspaceID, DefinitionID string
	Version                       int64
	Payload                       []byte `gorm:"type:jsonb"`
}

func decode(v row) (*d.Binding, error) {
	var out d.Binding
	err := json.Unmarshal(v.Payload, &out)
	return &out, err
}
func (r *Repository) Get(ctx context.Context, id string, lock bool) (*d.Binding, error) {
	db := dal.FromContext(ctx, r.db).Table("media_plugin.workspace_plugin_bindings").Where("id=?", id)
	if lock {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var v row
	err := db.Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return decode(v)
}
func (r *Repository) Version(ctx context.Context, id string, version int64) (*d.Binding, error) {
	var v row
	err := dal.FromContext(ctx, r.db).Table("media_plugin.plugin_binding_versions").Where("id=? AND version=?", id, version).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return decode(v)
}
func (r *Repository) read(ctx context.Context, table, field, value string) ([]d.Binding, error) {
	var rows []row
	err := dal.FromContext(ctx, r.db).Table(table).Where(field+"=?", value).Order("id,version DESC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]d.Binding, 0, len(rows))
	for _, v := range rows {
		out, err := decode(v)
		if err != nil {
			return nil, err
		}
		result = append(result, *out)
	}
	return result, nil
}
func (r *Repository) List(ctx context.Context, ws string) ([]d.Binding, error) {
	return r.read(ctx, "media_plugin.workspace_plugin_bindings", "workspace_id", ws)
}
func (r *Repository) Versions(ctx context.Context, id string) ([]d.Binding, error) {
	return r.read(ctx, "media_plugin.plugin_binding_versions", "id", id)
}
func (r *Repository) Save(ctx context.Context, v *d.Binding, expected int64, credential *d.Credential) error {
	db := dal.FromContext(ctx, r.db)
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	current := row{ID: v.ID, WorkspaceID: v.WorkspaceID, DefinitionID: v.DefinitionID, Version: v.Version, Payload: payload}
	var result *gorm.DB
	if expected == 0 {
		result = db.Table("media_plugin.workspace_plugin_bindings").Clauses(clause.OnConflict{DoNothing: true}).Create(&current)
	} else {
		result = db.Table("media_plugin.workspace_plugin_bindings").Where("id=? AND version=?", v.ID, expected).Updates(map[string]any{"version": v.Version, "payload": payload})
	}
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return d.ErrConflict
	}
	if credential != nil {
		if err = db.Table("media_plugin.mcp_credentials").Create(map[string]any{"id": credential.ID, "binding_id": v.ID, "ciphertext": credential.Ciphertext}).Error; err != nil {
			return err
		}
	}
	// The historical table stores only the immutable non-secret descriptor.
	return db.Table("media_plugin.plugin_binding_versions").Create(map[string]any{"id": v.ID, "workspace_id": v.WorkspaceID, "version": v.Version, "payload": payload}).Error
}
func (r *Repository) Credential(ctx context.Context, id, binding string) ([]byte, error) {
	var v struct{ Ciphertext []byte }
	err := dal.FromContext(ctx, r.db).Table("media_plugin.mcp_credentials").Where("id=? AND binding_id=?", id, binding).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	return v.Ciphertext, err
}
