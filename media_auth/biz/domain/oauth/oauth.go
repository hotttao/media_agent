// Package oauth 存放 oauth（OAuth2 Authorization Server）领域的稳定业务核心：Client、
// AuthCode、RefreshToken 值对象、仓库接口、不变量、domain 错误。
//
// 禁止 import Hertz / hertz_gen / biz/dal / biz/infra / logger / config / ORM / JWT SDK。
// 不变量：auth_code 一次性（消费即删）；refresh_token 轮转（旧删新发）；
//
//	redirect_uri 精确匹配；PKCE 仅 S256。
package oauth

import (
	"context"
	"time"
)

// Client 是 OAuth2 客户端注册（oauth_clients 行的领域形态）。
type Client struct {
	ID           string
	Name         string
	SecretHash   string   // 空串 = 公开 client（无 secret）
	RedirectURIs []string // 逐元素精确匹配
	CreatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// IsConfidential 是否机密 client（有 secret，token-exchange 须校验）。
func (c Client) IsConfidential() bool { return c.SecretHash != "" }

// AuthCode 是一次性授权码（oauth_authorization_codes 行的领域形态）。
type AuthCode struct {
	Code          string
	ClientID      string
	UserID        string
	RedirectURI   string
	CodeChallenge string // 空串 = 无 PKCE
	Scope         string
	ExpiresAt     time.Time
	CreatedAt     time.Time
}

// HasPKCE 是否携带 PKCE challenge。
func (a AuthCode) HasPKCE() bool { return a.CodeChallenge != "" }

// RefreshToken 是刷新令牌记录（oauth_refresh_tokens 行的领域形态）。
// TokenHash = SHA-256(明文)，明文不落库。
type RefreshToken struct {
	TokenHash string
	ClientID  string
	UserID    string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// User 是 oauth domain 对用户的视角（access token claims 所需字段）。
//
// 与 domain/auth.User 隔离（domain 同层不可互 import）；由 repo 从 users 表读取填充。
type User struct {
	ID          string
	Username    string
	DisplayName string
	Email       string // 空串 = NULL
	Role        string
}

// OAuthRepo 是 oauth 领域的仓储接口（由 dal/repo/oauth 实现）。
type OAuthRepo interface {
	// GetClient 按 id 查 client。不存在返回 ErrClientNotFound。
	GetClient(ctx context.Context, id string) (Client, error)
	// CreateAuthCode 插入授权码（code/scope/expires_at 由 service 生成）。
	CreateAuthCode(ctx context.Context, ac AuthCode) error
	// ConsumeAuthCode 消费授权码：DELETE WHERE code AND expires_at>now RETURNING。
	// 不存在/已过期返回 ErrAuthCodeNotFound。
	ConsumeAuthCode(ctx context.Context, code string) (AuthCode, error)
	// CreateRefreshToken 存刷新令牌（tokenHash 由 service 算）。
	CreateRefreshToken(ctx context.Context, rt RefreshToken) error
	// ConsumeRefreshToken 消费刷新令牌：DELETE WHERE token_hash AND expires_at>now RETURNING。
	// 不存在/已过期返回 ErrRefreshTokenNotFound。
	ConsumeRefreshToken(ctx context.Context, tokenHash string) (RefreshToken, error)
	// GetUserByID 按 id 查 user（access token claims 用）。不存在返回 ErrUserNotFound。
	GetUserByID(ctx context.Context, id string) (User, error)
	// CleanupExpired 删过期 auth_code + refresh_token（每小时调一次）。
	CleanupExpired(ctx context.Context) error
}

// domain 错误。
var (
	ErrClientNotFound       = errSentinel("client not found")
	ErrAuthCodeNotFound     = errSentinel("authorization code not found or expired")
	ErrRefreshTokenNotFound = errSentinel("refresh token not found or expired")
	ErrUserNotFound         = errSentinel("user not found")
)

// errSentinel 是简单的哨兵错误类型，支持 errors.Is 比较。
type errSentinel string

func (e errSentinel) Error() string { return string(e) }
