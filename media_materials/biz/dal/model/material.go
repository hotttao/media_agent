package model

import "time"

// Material 对应 PG media_materials.materials 表（curated 原料）。
type Material struct {
	ID             string     `gorm:"column:id;primaryKey"`
	OrganizationID string     `gorm:"column:organization_id;not null"`
	Scope          string     `gorm:"column:scope;not null"`
	Name           string     `gorm:"column:name;not null"`
	Description    string     `gorm:"column:description"`
	Prompt         string     `gorm:"column:prompt"`
	AssetKind      string     `gorm:"column:asset_kind;not null"`
	ObjectPath     string     `gorm:"column:object_path"`
	Content        string     `gorm:"column:content"`
	Metadata       string     `gorm:"column:metadata;type:jsonb"`
	OwnerUserID    string     `gorm:"column:owner_user_id;not null"`
	Visibility     string     `gorm:"column:visibility;not null;default:private"`
	TeamID         string     `gorm:"column:team_id"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;not null"`
	DeletedAt      *time.Time `gorm:"column:deleted_at"`
}

func (Material) TableName() string { return "media_materials.materials" }

// MaterialBinding 对应 media_materials.material_bindings 表。
type MaterialBinding struct {
	MaterialID    string     `gorm:"column:material_id;primaryKey"`
	OwnerKind     string     `gorm:"column:owner_kind;primaryKey"`
	OwnerID       string     `gorm:"column:owner_id;primaryKey"`
	BoundBy       string     `gorm:"column:bound_by;not null"`
	BoundAt       time.Time  `gorm:"column:bound_at;not null"`
	BindingsEndAt *time.Time `gorm:"column:bindings_end_at"`
}

func (MaterialBinding) TableName() string { return "media_materials.material_bindings" }
