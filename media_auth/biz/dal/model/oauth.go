// Package model 存放 media_auth 的 GORM row model（DB 行模型，承载 GORM tag）。
package model

import (
	"time"

	"github.com/lib/pq"
)

// OAuthClient 是 media_auth.oauth_clients 表的 GORM row model。
//
// redirect_uris: text[]，逐元素精确匹配(OAuth2 §3.1.2)。
// secret_hash: SHA-256(client_secret)；空 = 公开 client(无需 secret)。
type OAuthClient struct {
	ID           string         `gorm:"column:id;primaryKey"`
	Name         string         `gorm:"column:name"`
	SecretHash   *string        `gorm:"column:secret_hash"`
	RedirectURIs pq.StringArray `gorm:"column:redirect_uris;type:text[]"`
	HomepageURL  *string        `gorm:"column:homepage_url"`
	Description  *string        `gorm:"column:description"`
	CreatedBy    string         `gorm:"column:created_by"`
	CreatedAt    time.Time      `gorm:"column:created_at"`
	UpdatedAt    time.Time      `gorm:"column:updated_at"`
}

// TableName 固定表名（与 migration 一致，含 media_auth schema 前缀）。
func (OAuthClient) TableName() string { return "media_auth.oauth_clients" }

// OAuthAuthorizationCode 是 media_auth.oauth_authorization_codes 表的 GORM row model。
// 一次性授权码(10min TTL，消费即 DELETE 防重放)；code_challenge 留 PKCE S256。
type OAuthAuthorizationCode struct {
	Code          string    `gorm:"column:code;primaryKey"`
	ClientID      string    `gorm:"column:client_id"`
	UserID        string    `gorm:"column:user_id"`
	RedirectURI   string    `gorm:"column:redirect_uri"`
	CodeChallenge *string   `gorm:"column:code_challenge"`
	Scope         string    `gorm:"column:scope"`
	ExpiresAt     time.Time `gorm:"column:expires_at"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

// TableName 固定表名。
func (OAuthAuthorizationCode) TableName() string { return "media_auth.oauth_authorization_codes" }

// OAuthRefreshToken 是 media_auth.oauth_refresh_tokens 表的 GORM row model。
// 30d TTL；消费即 DELETE(轮转)；token_hash = SHA-256(refresh_token) 作 PK。
type OAuthRefreshToken struct {
	TokenHash string    `gorm:"column:token_hash;primaryKey"`
	ClientID  string    `gorm:"column:client_id"`
	UserID    string    `gorm:"column:user_id"`
	ExpiresAt time.Time `gorm:"column:expires_at"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName 固定表名。
func (OAuthRefreshToken) TableName() string { return "media_auth.oauth_refresh_tokens" }
