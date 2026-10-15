package mcp_catalog

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"media_agent/media_agent_gateway/biz/dal"
	"media_agent/media_agent_gateway/biz/dal/model"
	d "media_agent/media_agent_gateway/biz/domain/mcp_catalog"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db} }
func value(row model.MCPDefinition) *d.Definition {
	return &d.Definition{ID: row.ID, OrganizationID: row.OrganizationID, ServiceName: row.ServiceName, Endpoint: row.Endpoint, Transport: row.Transport, AuthenticationType: row.AuthenticationType, Status: row.Status, Version: row.Version, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
func (r *Repository) Get(ctx context.Context, id string, lock bool) (*d.Definition, error) {
	db := dal.FromContext(ctx, r.db).Where("id=?", id)
	if lock {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row model.MCPDefinition
	e := db.Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	return value(row), nil
}
func (r *Repository) Version(ctx context.Context, id string, version int64) (*d.Definition, error) {
	var row model.MCPDefinition
	e := dal.FromContext(ctx, r.db).Table("media_agent_gateway.mcp_definition_versions").Where("id=? AND version=?", id, version).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	return value(row), nil
}
func (r *Repository) List(ctx context.Context, after string, limit int) ([]d.Definition, error) {
	var rows []model.MCPDefinition
	e := dal.FromContext(ctx, r.db).Where("id>?", after).Order("id").Limit(limit).Find(&rows).Error
	result := make([]d.Definition, 0, len(rows))
	for _, row := range rows {
		result = append(result, *value(row))
	}
	return result, e
}
func (r *Repository) Save(ctx context.Context, v *d.Definition, expected int64) error {
	db := dal.FromContext(ctx, r.db)
	row := model.MCPDefinition{ID: v.ID, OrganizationID: v.OrganizationID, ServiceName: v.ServiceName, Endpoint: v.Endpoint, Transport: v.Transport, AuthenticationType: v.AuthenticationType, Status: v.Status, Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
	var result *gorm.DB
	if expected == 0 {
		result = db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	} else {
		result = db.Model(&model.MCPDefinition{}).Where("id=? AND version=?", v.ID, expected).Updates(map[string]any{"service_name": v.ServiceName, "endpoint": v.Endpoint, "transport": v.Transport, "authentication_type": v.AuthenticationType, "status": v.Status, "version": v.Version, "updated_at": v.UpdatedAt})
	}
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return d.ErrConflict
	}
	return db.Table("media_agent_gateway.mcp_definition_versions").Create(&row).Error
}
func (r *Repository) Changed(ctx context.Context, event d.Event) error {
	db := dal.FromContext(ctx, r.db)
	if e := db.Table("media_agent_gateway.mcp_catalog_events").Create(map[string]any{"id": event.ID, "definition_id": event.DefinitionID, "version": event.Version, "created_at": event.CreatedAt}).Error; e != nil {
		return e
	}
	return db.Exec("UPDATE media_agent_gateway.mcp_catalog_revision SET version=version+1 WHERE singleton=true").Error
}

func (r *Repository) PendingEvents(ctx context.Context, limit int) ([]d.Event, error) {
	var result []d.Event
	e := dal.FromContext(ctx, r.db).Raw(`WITH picked AS (SELECT id FROM media_agent_gateway.mcp_catalog_events WHERE delivered_at IS NULL AND available_at<=now() ORDER BY created_at LIMIT ? FOR UPDATE SKIP LOCKED) UPDATE media_agent_gateway.mcp_catalog_events e SET available_at=now()+interval '30 seconds',attempts=attempts+1 FROM picked WHERE e.id=picked.id RETURNING e.id,e.definition_id,e.version,e.created_at`, limit).Scan(&result).Error
	return result, e
}
func (r *Repository) CompleteEvent(ctx context.Context, id string) error {
	return dal.FromContext(ctx, r.db).Exec("UPDATE media_agent_gateway.mcp_catalog_events SET delivered_at=now() WHERE id=?", id).Error
}
func (r *Repository) DeferEvent(ctx context.Context, id string) error {
	return dal.FromContext(ctx, r.db).Exec("UPDATE media_agent_gateway.mcp_catalog_events SET available_at=now()+interval '5 seconds' WHERE id=? AND delivered_at IS NULL", id).Error
}

func (r *Repository) ListVisible(ctx context.Context, organization, after string, limit int) ([]d.Definition, error) {
	var rows []model.MCPDefinition
	db := dal.FromContext(ctx, r.db).Where("id>?", after)
	if organization != "" {
		db = db.Where("organization_id = '' OR organization_id = ?", organization)
	} else {
		db = db.Where("organization_id = ''")
	}
	err := db.Order("id").Limit(limit).Find(&rows).Error
	result := make([]d.Definition, 0, len(rows))
	for _, row := range rows {
		result = append(result, *value(row))
	}
	return result, err
}
