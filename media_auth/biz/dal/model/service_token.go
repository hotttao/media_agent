// Package model 存放 media_auth 的 GORM row model（DB 行模型，承载 GORM tag）。
package model

import "time"

// ServiceToken 是 media_auth.service_tokens 表的 GORM row model。
//
// token_hash: SHA-256(token)，DB 不存明文。
// token: 仅 platform token 落明文(getPlatformToken 用)；非 platform 必为 NULL。
// revoked_at: 非空 = 失效；列表过滤 revoked_at IS NULL。
// is_platform: 系统级 platform 令牌；登录后 ensurePlatformToken 自动建。
type ServiceToken struct {
	ID         string     `gorm:"column:id;primaryKey"`
	Name       string     `gorm:"column:name"`
	TokenHash  string     `gorm:"column:token_hash"`
	Token      *string    `gorm:"column:token"`
	CreatedBy  *string    `gorm:"column:created_by"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	RevokedAt  *time.Time `gorm:"column:revoked_at"`
	IsPlatform bool       `gorm:"column:is_platform"`
}

// TableName 固定表名（与 migration 一致，含 media_auth schema 前缀）。
func (ServiceToken) TableName() string { return "media_auth.service_tokens" }
