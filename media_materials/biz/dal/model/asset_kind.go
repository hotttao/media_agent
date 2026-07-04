package model

import "time"

// AssetKind 对应 PG media_materials.asset_kinds 表（资源类别注册表，全局无 scope）。
// name / resource_type / media_type / source_table 创建后锁定（service 层校验不可改）。
type AssetKind struct {
	Name         string    `gorm:"column:name;primaryKey"`
	ResourceType string    `gorm:"column:resource_type;not null"`
	MediaType    string    `gorm:"column:media_type;not null"`
	SourceTable  string    `gorm:"column:source_table;not null"`
	Description  string    `gorm:"column:description"`
	Status       string    `gorm:"column:status;not null;default:ENABLED"`
	CreatedAt    time.Time `gorm:"column:created_at;not null"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null"`
}

func (AssetKind) TableName() string { return "media_materials.asset_kinds" }
