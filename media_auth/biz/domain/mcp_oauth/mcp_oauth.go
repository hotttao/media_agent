// Package mcpoauth 存放 mcp_oauth（反向 OAuth client 连第三方 MCP server）领域的稳定业务核心：
// McpClient / McpToken 值对象、仓库接口、McpServerClient 接口、刷新失败宽限算法常量、domain 错误。
//
// 禁止 import Hertz / hertz_gen / biz/dal / biz/infra / logger / config / ORM / HTTP SDK。
// 不变量：pendingAuths 一次性；(user_id, server_origin) upsert 幂等；宽限算法见 policy。
package mcpoauth

import (
	"context"
	"encoding/json"
	"time"
)

// 刷新失败宽限算法常量（对齐 control-plane services/mcp-oauth.ts）。
const (
	RefreshFailGraceSeconds = 600 // 首次失败距今 >= 600s 才考虑删 token
	RefreshFailMinCount     = 3   // 且连续失败 >= 3 次
	DefaultTokenTTLSeconds  = 1800
)

// Metadata 是 MCP server /.well-known/oauth-authorization-server 响应（缓存于 mcp_oauth_clients.metadata）。
type Metadata struct {
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	RegistrationEndpoint  string   `json:"registration_endpoint,omitempty"`
	ScopesSupported       []string `json:"scopes_supported,omitempty"`
}

// McpClient 是 mcp_oauth_clients 行的领域形态（某 MCP server 的 OAuth client 注册）。
type McpClient struct {
	ServerOrigin string
	Metadata     Metadata // 解析后的 metadata（DB 存 jsonb）
	ClientID     string
	ClientSecret string // 空串 = 公开 client
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// McpToken 是 mcp_oauth_tokens 行的领域形态（user 对某 MCP server 的 token）。
//
// AccessToken/RefreshToken 明文存（对 media_auth 不透明）。RefreshFailCount + RefreshFailFirstAt
// 支撑刷新失败宽限算法（policy.ShouldDropToken）。
type McpToken struct {
	UserID             string
	ServerOrigin       string
	AccessToken        string
	RefreshToken       string // 空串 = 无 refresh
	TokenType          string
	Scope              string
	ExpiresAt          *time.Time // nil = 未知，回退 updated_at + DefaultTokenTTL
	RefreshFailCount   int
	RefreshFailFirstAt *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// TokenResponse 是 token_endpoint 的响应（exchange code / refresh 都用）。
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// McpServerClient 是第三方 MCP server OAuth 端点的稳定接口（由 shared/client/mcp 实现）。
//
// domain 定义接口，实现在 shared（依赖 HTTP client）；service/workflow 经此调用 MCP server，
// 不直接 import HTTP SDK。
type McpServerClient interface {
	// DiscoverMetadata GET {origin}/.well-known/oauth-authorization-server。
	// 返回 (metadata, true, nil) 当 server 支持 OAuth；不支持 → (Metadata{}, false, nil)；不可达 → error。
	DiscoverMetadata(ctx context.Context, serverURL string) (Metadata, string, bool, error)
	// RegisterClient 动态注册 client（若 metadata 提供 registration_endpoint）。
	RegisterClient(ctx context.Context, client McpClient, callbackURL string) (McpClient, error)
	// ExchangeCode 用 code 换 token。
	ExchangeCode(ctx context.Context, client McpClient, code, codeVerifier, callbackURL string) (TokenResponse, error)
	// RefreshToken 用 refresh_token 换新 token。permanent=true 表示 refresh_token/client 已死。
	RefreshToken(ctx context.Context, client McpClient, refreshToken string) (resp TokenResponse, permanent bool, err error)
}

// McpRepo 是 mcp_oauth 领域的仓储接口（由 dal/repo/mcp_oauth 实现）。
type McpRepo interface {
	// UpsertClient upsert mcp_oauth_clients（按 server_origin）。
	UpsertClient(ctx context.Context, c McpClient) error
	// GetClient 按 server_origin 查 client。不存在返回 ErrClientNotFound。
	GetClient(ctx context.Context, serverOrigin string) (McpClient, error)
	// UpsertToken upsert mcp_oauth_tokens（按 user_id+server_origin），重置 refresh_fail 计数。
	UpsertToken(ctx context.Context, userID, serverOrigin string, resp TokenResponse, ttlSeconds int) error
	// GetToken 取 user 对 server_origin 的 token。不存在返回 ErrTokenNotFound。
	GetToken(ctx context.Context, userID, serverOrigin string) (McpToken, error)
	// ListTokensForUser 列 user 的全部 token。
	ListTokensForUser(ctx context.Context, userID string) ([]McpToken, error)
	// RecordRefreshFailure 失败计数 +1，首次失败盖章。返回当前计数 + 首次时间。
	RecordRefreshFailure(ctx context.Context, userID, serverOrigin string) (failCount int, firstFailAt *time.Time, err error)
	// DeleteToken 删 token。
	DeleteToken(ctx context.Context, userID, serverOrigin string) error
}

// domain 错误。
var (
	ErrClientNotFound = errSentinel("mcp oauth client not found")
	ErrTokenNotFound  = errSentinel("mcp oauth token not found")
)

// errSentinel 是简单的哨兵错误类型，支持 errors.Is 比较。
type errSentinel string

func (e errSentinel) Error() string { return string(e) }

// MetadataJSON 把 Metadata 序列化为 jsonb 存储（repo 用）。
func MetadataJSON(m Metadata) json.RawMessage {
	b, _ := json.Marshal(m)
	return b
}

// ParseMetadata 从 jsonb 解析 Metadata（repo 用）。
func ParseMetadata(raw json.RawMessage) Metadata {
	var m Metadata
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}
