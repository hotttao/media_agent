package model

import "time"

// Product 对应 PG media_materials.products 表。
type Product struct {
	ID               string     `gorm:"column:id;primaryKey"`
	OrganizationID   string     `gorm:"column:organization_id;not null"`
	Scope            string     `gorm:"column:scope;not null"`
	Name             string     `gorm:"column:name;not null"`
	Description      string     `gorm:"column:description"`
	TargetAudience   string     `gorm:"column:target_audience"`
	ProductDetails   string     `gorm:"column:product_details"`
	DisplayActions   string     `gorm:"column:display_actions"`
	Tags             string     `gorm:"column:tags;type:jsonb"`
	ExternalCommerce string     `gorm:"column:external_commerce;type:jsonb"`
	Metadata         string     `gorm:"column:metadata;type:jsonb"`
	Status           string     `gorm:"column:status;not null;default:active"`
	CreatedBy        string     `gorm:"column:created_by;not null"`
	UpdatedBy        string     `gorm:"column:updated_by"`
	CreatedAt        time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt        time.Time  `gorm:"column:updated_at;not null"`
	DeletedAt        *time.Time `gorm:"column:deleted_at"`
}

func (Product) TableName() string { return "media_materials.products" }

type ProductImage struct {
	ID             string    `gorm:"column:id;primaryKey"`
	OrganizationID string    `gorm:"column:organization_id;not null"`
	ProductID      string    `gorm:"column:product_id;not null"`
	ObjectPath     string    `gorm:"column:object_path;not null"`
	IsMain         bool      `gorm:"column:is_main;not null"`
	SortOrder      int       `gorm:"column:sort_order;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null"`
}

func (ProductImage) TableName() string { return "media_materials.product_images" }
