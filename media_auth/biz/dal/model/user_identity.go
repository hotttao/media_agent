// Package model 存放 media_auth 的 GORM row model（DB 行模型，承载 GORM tag）。
// 禁止被 service/workflow/policy/domain 直接 import；仅 dal/repo 使用。
package model

import (
	"time"

	"gorm.io/datatypes"
)

// UserIdentity 是 media_auth.user_identities 表的 GORM row model。
//
// 外部身份绑定：(provider, external_id) 全局唯一(PK)；(user_id, provider) 每用户每 provider 至多一条(唯一索引)。
// metadata jsonb 存 provider-specific 字段(企微 avatar/email 等)。
type UserIdentity struct {
	UserID      string         `gorm:"column:user_id;primaryKey"`
	Provider    string         `gorm:"column:provider;primaryKey"`
	ExternalID  string         `gorm:"column:external_id"`
	DisplayName *string        `gorm:"column:display_name"`
	Metadata    datatypes.JSON `gorm:"column:metadata"`
	CreatedAt   time.Time      `gorm:"column:created_at"`
}

// TableName 固定表名（与 migration 一致，含 media_auth schema 前缀）。
func (UserIdentity) TableName() string { return "media_auth.user_identities" }
