package model

import "time"

// GeneratedAsset 对应 PG media_materials.generated_assets 表。
// job_id / step_run_id 跨库指向 media_workflow（无 FK）。
type GeneratedAsset struct {
	ID                string     `gorm:"column:id;primaryKey"`
	OrganizationID    string     `gorm:"column:organization_id;not null"`
	Scope             string     `gorm:"column:scope;not null"`
	Name              string     `gorm:"column:name;not null"`
	AssetKind         string     `gorm:"column:asset_kind;not null"`
	URL               string     `gorm:"column:url"`
	ObjectPath        string     `gorm:"column:object_path"`
	Content           string     `gorm:"column:content"`
	OwnerUserID       string     `gorm:"column:owner_user_id;not null"`
	Visibility        string     `gorm:"column:visibility;not null;default:private"`
	TeamID            string     `gorm:"column:team_id"`
	JobID             *string    `gorm:"column:job_id"`
	StepRunID         *string    `gorm:"column:step_run_id"`
	TemplateID        *string    `gorm:"column:template_id"`
	TemplateVersion   *int       `gorm:"column:template_version"`
	Origin            string     `gorm:"column:origin;not null;default:native"`
	SourceSystem      *string    `gorm:"column:source_system"`
	SourceRecordID    *string    `gorm:"column:source_record_id"`
	SourceSHA256      *string    `gorm:"column:source_sha256"`
	SourceCreatedAt   *time.Time `gorm:"column:source_created_at"`
	ImportedAt        *time.Time `gorm:"column:imported_at"`
	SourceVirtualIPID string     `gorm:"column:source_virtual_ip_id"`
	SourceProductID   string     `gorm:"column:source_product_id"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null"`
	DeletedAt         *time.Time `gorm:"column:deleted_at"`
}

func (GeneratedAsset) TableName() string { return "media_materials.generated_assets" }
