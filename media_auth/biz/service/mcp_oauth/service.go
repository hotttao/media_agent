// Package mcpoauth 是 mcp_oauth 用例服务层（单 domain 编排 + 外部 MCP server 调用）。
//
// 接受 command，编排 domain/policy/repo + McpServerClient + pendingauth.Store 完成用例；
// 禁止 import Hertz / hertz_gen / biz/dal / 其它 service 包。
// 对齐 control-plane routes/mcp-oauth.ts（discover/status/authorize/disconnect）+ services/mcp-oauth.ts。
// callback 编排（exchange code + notifyAgentReload）归 workflow/mcp_oauth_connect。
package mcpoauth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	domainmcp "media_agent/media_auth/biz/domain/mcp_oauth"
	policymcp "media_agent/media_auth/biz/policy/mcp_oauth"
	"media_agent/media_auth/biz/shared/client/mcp"
	"media_agent/media_auth/biz/shared/pendingauth"
	"media_agent/media_auth/biz/shared/token"
)

// Service 是 mcp_oauth 用例服务。
type Service struct {
	repo   domainmcp.McpRepo
	client *mcp.Client
	store  *pendingauth.Store
}

// NewService 构造 mcp_oauth 服务。
func NewService(repo domainmcp.McpRepo, client *mcp.Client, store *pendingauth.Store) *Service {
	return &Service{repo: repo, client: client, store: store}
}

// 用例错误。
var (
	ErrURLRequired       = errors.New("url is required")
	ErrServerOriginReq   = errors.New("server_origin and workspace_id are required")
	ErrDiscoveryFailed   = errors.New("cannot reach MCP server")
	ErrOAuthNotSupported = errors.New("OAuth not supported by this server")
	ErrReauthRequired    = errors.New("mcp oauth reauthorization required")
)

// DiscoverResult 是 DiscoverOAuth 返回。
type DiscoverResult struct {
	OAuthRequired bool
	ServerOrigin  string
}

// DiscoverOAuth 探测 MCP server 是否需要 OAuth（POST /api/mcp-oauth/discover）。
func (s *Service) DiscoverOAuth(ctx context.Context, serverURL string) (DiscoverResult, error) {
	if serverURL == "" {
		return DiscoverResult{}, ErrURLRequired
	}
	_, origin, ok, err := s.client.DiscoverMetadata(ctx, serverURL)
	if err != nil {
		return DiscoverResult{}, ErrDiscoveryFailed
	}
	if !ok {
		return DiscoverResult{OAuthRequired: false}, nil
	}
	// 缓存 client 元数据：discover 命中即 upsert metadata（client_id 留待 register 时填）。
	// control-plane 现状 discover 仅返回 origin，register 在 authorize 时进行；此处保持一致。
	return DiscoverResult{OAuthRequired: true, ServerOrigin: origin}, nil
}

// ConnectionStatusEntry 是 GetConnectionStatus 单条状态。
type ConnectionStatusEntry struct {
	Connected bool
	ExpiresAt string // RFC3339，空=未连接
}

// GetConnectionStatus 查 user 对各 MCP server 的连接状态（GET /api/mcp-oauth/status）。
func (s *Service) GetConnectionStatus(ctx context.Context, userID string, servers []string) (map[string]ConnectionStatusEntry, error) {
	out := make(map[string]ConnectionStatusEntry, len(servers))
	if len(servers) == 0 {
		return out, nil
	}
	tokens, err := s.repo.ListTokensForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	byOrigin := make(map[string]domainmcp.McpToken, len(tokens))
	for _, t := range tokens {
		byOrigin[t.ServerOrigin] = t
	}
	for _, origin := range servers {
		if origin == "" {
			continue
		}
		if t, ok := byOrigin[origin]; ok {
			entry := ConnectionStatusEntry{Connected: true}
			if t.ExpiresAt != nil {
				entry.ExpiresAt = t.ExpiresAt.Format(time.RFC3339)
			}
			out[origin] = entry
		} else {
			out[origin] = ConnectionStatusEntry{Connected: false}
		}
	}
	return out, nil
}

// StartAuthorization 启动授权：discover → getOrRegisterClient → build auth URL（GET /api/mcp-oauth/authorize）。
// 返回重定向 URL（handler c.Redirect）。callbackURL = media_auth origin + /api/mcp-oauth/callback。
func (s *Service) StartAuthorization(ctx context.Context, userID, serverOrigin, workspaceID, callbackURL string) (string, error) {
	if serverOrigin == "" || workspaceID == "" {
		return "", ErrServerOriginReq
	}
	metadata, _, ok, err := s.client.DiscoverMetadata(ctx, serverOrigin)
	if err != nil {
		return "", ErrDiscoveryFailed
	}
	if !ok {
		return "", ErrOAuthNotSupported
	}
	client, err := s.getOrRegisterClient(ctx, serverOrigin, metadata, callbackURL)
	if err != nil {
		return "", err
	}
	redirectURL, err := s.buildAuthorizationURL(client, userID, workspaceID, callbackURL)
	if err != nil {
		return "", err
	}
	return redirectURL, nil
}

// Disconnect 删 user 对 server_origin 的 token（DELETE /api/mcp-oauth/:server_origin）。
func (s *Service) Disconnect(ctx context.Context, userID, serverOrigin string) error {
	return s.repo.DeleteToken(ctx, userID, serverOrigin)
}

// ConsumePendingAuth 消费 pending state（workflow HandleCallback 用）。一次性。
func (s *Service) ConsumePendingAuth(state string) (pendingauth.PendingAuth, bool) {
	return s.store.Consume(state)
}

// GetClient 取 MCP client（workflow HandleCallback 用）。
func (s *Service) GetClient(ctx context.Context, serverOrigin string) (domainmcp.McpClient, error) {
	return s.repo.GetClient(ctx, serverOrigin)
}

// EnsureClient 重新 discover + 取/注册 client（workflow HandleCallback 用）。
// control-plane callback 路径会 re-discover metadata 后取已注册 client。
func (s *Service) EnsureClient(ctx context.Context, serverOrigin, callbackURL string) (domainmcp.McpClient, error) {
	// 先查 DB（authorize 时已注册）。
	if existing, err := s.repo.GetClient(ctx, serverOrigin); err == nil {
		return existing, nil
	} else if !errors.Is(err, domainmcp.ErrClientNotFound) {
		return domainmcp.McpClient{}, err
	}
	// 不存在 → re-discover + register（极少走到，正常流程 authorize 已注册）。
	metadata, _, ok, err := s.client.DiscoverMetadata(ctx, serverOrigin)
	if err != nil || !ok {
		return domainmcp.McpClient{}, fmt.Errorf("mcp oauth: re-discover failed for %s", serverOrigin)
	}
	return s.getOrRegisterClient(ctx, serverOrigin, metadata, callbackURL)
}

// UpsertToken upsert token（workflow HandleCallback 用）。
func (s *Service) UpsertToken(ctx context.Context, userID, serverOrigin string, resp domainmcp.TokenResponse) error {
	return s.repo.UpsertToken(ctx, userID, serverOrigin, resp, domainmcp.DefaultTokenTTLSeconds)
}

// ExchangeCode 用 code 换 token（workflow HandleCallback 用）。
func (s *Service) ExchangeCode(ctx context.Context, client domainmcp.McpClient, code, codeVerifier, callbackURL string) (domainmcp.TokenResponse, error) {
	return s.client.ExchangeCode(ctx, client, code, codeVerifier, callbackURL)
}

// ── 内部：getValidAccessToken（刷新失败宽限算法） ────────────────────

// ErrReauthRequiredVar 暴露 reauth 错误（control_plane mcp-proxy 调用时据此提示重连）。
var ErrReauthRequiredVar = ErrReauthRequired

// GetValidAccessToken 取有效 access token，过期则 refresh；带刷新失败宽限算法。
//
// 流程（对齐 control-plane getValidAccessToken）：
//  1. 取 token 行；无 → ("", nil)
//  2. 未过期且非 force → 返回 access
//  3. 无 refresh_token → ("", nil)
//  4. refresh：成功 → upsert（重置计数）→ 返回新 access
//  5. refresh 失败且 permanent → 计数 +1；超过宽限（>=3 次 且 >=600s）→ 删 token + ErrReauthRequired；否则 ("", nil) 暂态
//  6. refresh 失败非 permanent → ("", nil) 暂态
func (s *Service) GetValidAccessToken(ctx context.Context, userID, serverOrigin string, forceRefresh bool) (string, error) {
	t, err := s.repo.GetToken(ctx, userID, serverOrigin)
	if err != nil {
		if errors.Is(err, domainmcp.ErrTokenNotFound) {
			return "", nil
		}
		return "", err
	}
	if !forceRefresh {
		effective := effectiveExpiry(t)
		if effective.After(time.Now()) {
			return t.AccessToken, nil
		}
	}
	if t.RefreshToken == "" {
		return "", nil
	}
	client, err := s.repo.GetClient(ctx, serverOrigin)
	if err != nil {
		return "", nil
	}
	resp, permanent, err := s.client.RefreshToken(ctx, client, t.RefreshToken)
	if err == nil {
		// refresh 成功 → upsert（重置计数）。
		if err := s.repo.UpsertToken(ctx, userID, serverOrigin, resp, resp.ExpiresIn); err != nil {
			return "", err
		}
		return resp.AccessToken, nil
	}
	if !permanent {
		// 暂态失败（5xx/网络）→ 返回 nil，下次重试。
		return "", nil
	}
	// permanent 失败 → 走宽限算法。
	failCount, firstFailAt, err := s.repo.RecordRefreshFailure(ctx, userID, serverOrigin)
	if err != nil {
		return "", nil
	}
	if policymcp.ShouldDropToken(failCount, firstFailAt, time.Now()) {
		_ = s.repo.DeleteToken(ctx, userID, serverOrigin)
		return "", fmt.Errorf("%w: %s", ErrReauthRequired, serverOrigin)
	}
	return "", nil // 宽限窗口内，暂态
}

// effectiveExpiry 计算有效过期时间：expires_at 非空用之，否则回退 updated_at + DefaultTokenTTL。
func effectiveExpiry(t domainmcp.McpToken) time.Time {
	if t.ExpiresAt != nil {
		return *t.ExpiresAt
	}
	return t.UpdatedAt.Add(domainmcp.DefaultTokenTTLSeconds * time.Second)
}

// ── 内部：client 注册 + 授权 URL ──────────────────────────────────────

// getOrRegisterClient 取已有 client 或动态注册。
func (s *Service) getOrRegisterClient(ctx context.Context, serverOrigin string, metadata domainmcp.Metadata, callbackURL string) (domainmcp.McpClient, error) {
	if existing, err := s.repo.GetClient(ctx, serverOrigin); err == nil {
		return existing, nil
	} else if !errors.Is(err, domainmcp.ErrClientNotFound) {
		return domainmcp.McpClient{}, err
	}
	// 不存在 → 注册。
	mc := domainmcp.McpClient{ServerOrigin: serverOrigin, Metadata: metadata}
	if metadata.RegistrationEndpoint != "" {
		registered, err := s.client.RegisterClient(ctx, mc, callbackURL)
		if err != nil {
			return domainmcp.McpClient{}, fmt.Errorf("mcp oauth: register client for %s: %w", serverOrigin, err)
		}
		mc = registered
	} else {
		return domainmcp.McpClient{}, fmt.Errorf("mcp oauth: no registration_endpoint for %s", serverOrigin)
	}
	if err := s.repo.UpsertClient(ctx, mc); err != nil {
		return domainmcp.McpClient{}, err
	}
	return mc, nil
}

// buildAuthorizationUrl 生成授权 URL + 存 pendingAuth。
func (s *Service) buildAuthorizationURL(client domainmcp.McpClient, userID, workspaceID, callbackURL string) (string, error) {
	codeVerifier, err := token.GeneratePKCEVerifier()
	if err != nil {
		return "", err
	}
	codeChallenge := token.ComputePKCEChallenge(codeVerifier)
	state, err := token.GenerateState()
	if err != nil {
		return "", err
	}
	s.store.Put(state, pendingauth.PendingAuth{
		UserID:       userID,
		ServerOrigin: client.ServerOrigin,
		CodeVerifier: codeVerifier,
		WorkspaceID:  workspaceID,
	})
	// 拼 authorize endpoint?response_type=code&...
	u, err := url.Parse(client.Metadata.AuthorizationEndpoint)
	if err != nil {
		return "", fmt.Errorf("mcp oauth: parse authorization_endpoint: %w", err)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", client.ClientID)
	q.Set("redirect_uri", callbackURL)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// GetUsableToken distinguishes a missing/revoked connection from a temporary
// refresh failure. Callers must not ask users to reconnect because Auth is down.
func (s *Service) GetUsableToken(ctx context.Context, userID, origin string) (string, error) {
	value, err := s.GetValidAccessToken(ctx, userID, origin, false)
	if err != nil || value != "" {
		return value, err
	}
	connection, err := s.repo.GetToken(ctx, userID, origin)
	if errors.Is(err, domainmcp.ErrTokenNotFound) {
		return "", ErrReauthRequired
	}
	if err != nil {
		return "", err
	}
	if connection.RefreshToken == "" {
		return "", ErrReauthRequired
	}
	return "", errors.New("MCP OAuth credential temporarily unavailable")
}
