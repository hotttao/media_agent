package model

import "time"

// Product 对应 PG media_materials.products 表。
type Product struct {
	ID             string     `gorm:"column:id;primaryKey"`
	Scope          string     `gorm:"column:scope;not null"`
	Name           string     `gorm:"column:name;not null"`
	Description    string     `gorm:"column:description"`
	TargetAudience string     `gorm:"column:target_audience"`
	ProductDetails string     `gorm:"column:product_details"`
	DisplayActions string     `gorm:"column:display_actions"`
	Status         string     `gorm:"column:status;not null;default:active"`
	CreatedBy      string     `gorm:"column:created_by;not null"`
	UpdatedBy      string     `gorm:"column:updated_by"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;not null"`
	DeletedAt      *time.Time `gorm:"column:deleted_at"`
}

func (Product) TableName() string { return "media_materials.products" }
