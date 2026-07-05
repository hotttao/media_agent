// Package model 存放 media_auth 的 GORM row model（DB 行模型，承载 GORM tag）。
package model

import (
	"time"

	"gorm.io/datatypes"
)

// McpOAuthClient 是 media_auth.mcp_oauth_clients 表的 GORM row model。
// server_origin 作主键(同一 MCP server 仅一个 client)；metadata 缓存 /.well-known/oauth-authorization-server。
type McpOAuthClient struct {
	ServerOrigin string         `gorm:"column:server_origin;primaryKey"`
	Metadata     datatypes.JSON `gorm:"column:metadata"`
	ClientID     string         `gorm:"column:client_id"`
	ClientSecret *string        `gorm:"column:client_secret"`
	CreatedAt    time.Time      `gorm:"column:created_at"`
	UpdatedAt    time.Time      `gorm:"column:updated_at"`
}

// TableName 固定表名（与 migration 一致，含 media_auth schema 前缀）。
func (McpOAuthClient) TableName() string { return "media_auth.mcp_oauth_clients" }

// McpOAuthToken 是 media_auth.mcp_oauth_tokens 表的 GORM row model。
//
// refresh 失败宽限(对齐控制面 migrations/117):
//
//	refresh_fail_count >= 3 且 refresh_fail_first_at 距今 >= 600s 才删 token;
//	之前视为暂态(getValidAccessToken 返回 nil);成功 refresh 重置两者。
//
// access_token / refresh_token 明文存(对 media_auth 不透明)。
type McpOAuthToken struct {
	UserID             string     `gorm:"column:user_id;primaryKey"`
	ServerOrigin       string     `gorm:"column:server_origin;primaryKey"`
	AccessToken        string     `gorm:"column:access_token"`
	RefreshToken       *string    `gorm:"column:refresh_token"`
	TokenType          string     `gorm:"column:token_type"`
	Scope              *string    `gorm:"column:scope"`
	ExpiresAt          *time.Time `gorm:"column:expires_at"`
	RefreshFailCount   int        `gorm:"column:refresh_fail_count"`
	RefreshFailFirstAt *time.Time `gorm:"column:refresh_fail_first_at"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
}

// TableName 固定表名。
func (McpOAuthToken) TableName() string { return "media_auth.mcp_oauth_tokens" }
