package model

import "time"

type Organization struct {
	ID        string    `gorm:"column:id;primaryKey"`
	Name      string    `gorm:"column:name"`
	Status    string    `gorm:"column:status"`
	Version   int64     `gorm:"column:version"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Organization) TableName() string { return "media_auth.organizations" }

type OrganizationMember struct {
	OrganizationID string    `gorm:"column:organization_id;primaryKey"`
	UserID         string    `gorm:"column:user_id;primaryKey"`
	Role           string    `gorm:"column:role"`
	JoinedAt       time.Time `gorm:"column:joined_at"`
}

func (OrganizationMember) TableName() string { return "media_auth.organization_members" }

type OrganizationEntitlement struct {
	OrganizationID string    `gorm:"column:organization_id;primaryKey"`
	FeatureKey     string    `gorm:"column:feature_key;primaryKey"`
	GrantedBy      string    `gorm:"column:granted_by"`
	GrantedAt      time.Time `gorm:"column:granted_at"`
}

func (OrganizationEntitlement) TableName() string { return "media_auth.organization_entitlements" }
