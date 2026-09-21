package model

import "time"

// VirtualIp 对应 PG media_materials.virtual_ips 表。
type VirtualIp struct {
	ID                  string     `gorm:"column:id;primaryKey"`
	OrganizationID      string     `gorm:"column:organization_id;not null"`
	Scope               string     `gorm:"column:scope;not null"`
	Name                string     `gorm:"column:name;not null"`
	Description         string     `gorm:"column:description"`
	AvatarObjectPath    string     `gorm:"column:avatar_object_path"`
	FullBodyObjectPath  string     `gorm:"column:full_body_object_path"`
	ThreeViewObjectPath string     `gorm:"column:three_view_object_path"`
	NineViewObjectPath  string     `gorm:"column:nine_view_object_path"`
	PersonaProfile      string     `gorm:"column:persona_profile;type:jsonb"`
	Status              string     `gorm:"column:status;not null;default:active"`
	Visibility          string     `gorm:"column:visibility;not null;default:private"`
	OwnerUserID         string     `gorm:"column:owner_user_id;not null"`
	CreatedAt           time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt           time.Time  `gorm:"column:updated_at;not null"`
	DeletedAt           *time.Time `gorm:"column:deleted_at"`
}

func (VirtualIp) TableName() string { return "media_materials.virtual_ips" }

// VirtualIpGrant 对应 media_materials.virtual_ip_grants 表。
type VirtualIpGrant struct {
	OrganizationID string    `gorm:"column:organization_id;not null"`
	VirtualIPID    string    `gorm:"column:virtual_ip_id;primaryKey"`
	TeamID         string    `gorm:"column:team_id;primaryKey"`
	Permission     string    `gorm:"column:permission;not null"`
	GrantedBy      string    `gorm:"column:granted_by;not null"`
	GrantedAt      time.Time `gorm:"column:granted_at;not null"`
}

func (VirtualIpGrant) TableName() string { return "media_materials.virtual_ip_grants" }
