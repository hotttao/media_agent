// Package model 存放 media_auth 的 GORM row model（DB 行模型，承载 GORM tag）。
package model

import "time"

// Team 是 media_auth.teams 表的 GORM row model。
// id 由应用层 generateId() 产生(短随机串)；created_by 为创建者(首任 admin)。
type Team struct {
	ID              string    `gorm:"column:id;primaryKey"`
	OrganizationID  string    `gorm:"column:organization_id"`
	Name            string    `gorm:"column:name"`
	Description     *string   `gorm:"column:description"`
	EntitlementMode string    `gorm:"column:entitlement_mode"`
	Status          string    `gorm:"column:status"`
	CreatedBy       string    `gorm:"column:created_by"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

// TableName 固定表名（与 migration 一致，含 media_auth schema 前缀）。
func (Team) TableName() string { return "media_auth.teams" }

// TeamMember 是 media_auth.team_members 表的 GORM row model。
// 复合主键 (team_id, user_id)；role: 'admin' | 'member'(DB CHECK)。
type TeamMember struct {
	TeamID   string    `gorm:"column:team_id;primaryKey"`
	UserID   string    `gorm:"column:user_id;primaryKey"`
	Role     string    `gorm:"column:role"`
	JoinedAt time.Time `gorm:"column:joined_at"`
}

// TableName 固定表名。
func (TeamMember) TableName() string { return "media_auth.team_members" }

// TeamInvite 是 media_auth.team_invites 表的 GORM row model。
// token = 'tinv_' + base64url(randomBytes(24))，PK；expires_at 可空(空=不过期)。
type TeamInvite struct {
	Token     string     `gorm:"column:token;primaryKey"`
	TeamID    string     `gorm:"column:team_id"`
	CreatedBy string     `gorm:"column:created_by"`
	ExpiresAt *time.Time `gorm:"column:expires_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

// TableName 固定表名。
func (TeamInvite) TableName() string { return "media_auth.team_invites" }
