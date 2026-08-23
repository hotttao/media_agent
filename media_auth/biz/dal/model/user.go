// Package model 存放 media_auth 的 GORM row model（DB 行模型，承载 GORM tag）。
// 禁止被 service/workflow/policy/domain 直接 import；仅 dal/repo 使用。
// schema 由 biz/dal/migrations 定义，row model 与之对齐；schema 变更 → 加 migration → 重跑 GORM Gen。
package model

import "time"

// User 是 media_auth.users 表的 GORM row model，对应 auth 领域 User 实体的持久化形态。
//
// 可空字段用指针（password_hash 空=LDAP/企微派生账号不可改密；email/default_prompt_id/last_login_at 可空）。
// id 由应用层 generateId() 产生（text 短随机），非自增。
type User struct {
	ID              string     `gorm:"column:id;primaryKey"`
	Username        string     `gorm:"column:username;not null"`
	DisplayName     string     `gorm:"column:display_name;not null"`
	Email           *string    `gorm:"column:email"`
	PasswordHash    *string    `gorm:"column:password_hash"`
	Role            string     `gorm:"column:role;not null;default:user"`
	DefaultPromptID *string    `gorm:"column:default_prompt_id"`
	AutoEvolution   bool       `gorm:"column:auto_evolution;not null;default:false"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	LastLoginAt     *time.Time `gorm:"column:last_login_at"`
	Status          string     `gorm:"column:status;not null;default:active"`
	AuthVersion     int64      `gorm:"column:auth_version;not null;default:1"`
	DisabledAt      *time.Time `gorm:"column:disabled_at"`
	DeletedAt       *time.Time `gorm:"column:deleted_at"`
	DeletedBy       *string    `gorm:"column:deleted_by"`
}

// TableName 固定表名（与 migration 一致，含 media_auth schema 前缀）。
func (User) TableName() string { return "media_auth.users" }
