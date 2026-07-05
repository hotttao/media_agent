// Package workflow — mcp_oauth_connect：MCP OAuth 回调编排。
//
// 消费 pendingAuth → 校验 user → 取/注册 client → code 换 token → upsert token →
// 返回 popup HTML（postMessage + close）。media_auth 不主动通知下游 reload（spec §8）。
package workflow

import (
	"context"
	"fmt"
	"html"
	"log"

	domainmcp "media_agent/media_auth/biz/domain/mcp_oauth"
	mcpoauthservice "media_agent/media_auth/biz/service/mcp_oauth"
	"media_agent/media_auth/biz/shared/pendingauth"
)

// McpOAuthService 是 workflow 依赖的 mcp_oauth service 接缝（*mcpoauthservice.Service 实现）。
// 抽接口便于单测注入 fake，不依赖真实 MCP server / DB。
type McpOAuthService interface {
	ConsumePendingAuth(state string) (pendingauth.PendingAuth, bool)
	EnsureClient(ctx context.Context, serverOrigin, callbackURL string) (domainmcp.McpClient, error)
	ExchangeCode(ctx context.Context, client domainmcp.McpClient, code, codeVerifier, callbackURL string) (domainmcp.TokenResponse, error)
	UpsertToken(ctx context.Context, userID, serverOrigin string, resp domainmcp.TokenResponse) error
}

// McpOAuthConnect 编排 MCP OAuth 回调。
type McpOAuthConnect struct {
	svc McpOAuthService
}

// NewMcpOAuthConnect 构造 MCP OAuth 回调 workflow。
// media_auth 不主动通知下游 reload（spec §8）；MCP 连接变更由消费方轮询/订阅。
func NewMcpOAuthConnect(svc *mcpoauthservice.Service) *McpOAuthConnect {
	return &McpOAuthConnect{svc: svc}
}

// McpCallbackResult 是 HandleCallback 返回。HTML 给 popup 页面渲染；HTTPStatus 由 handler 写。
type McpCallbackResult struct {
	HTML       string
	HTTPStatus int
}

// HandleCallback 处理 MCP OAuth 回调。
//
// 入参 callbackURL = media_auth origin + /api/mcp-oauth/callback（handler 从请求拼）。
// control-plane 现状：所有错误返回 errorPage HTML（非 JSON）。
func (w *McpOAuthConnect) HandleCallback(ctx context.Context, userID, code, state, errParam, callbackURL string) McpCallbackResult {
	if errParam != "" {
		return McpCallbackResult{HTML: errorPage(fmt.Sprintf("Authorization denied: %s", errParam)), HTTPStatus: 200}
	}
	if code == "" || state == "" {
		return McpCallbackResult{HTML: errorPage("Missing code or state parameter"), HTTPStatus: 400}
	}
	pending, ok := w.svc.ConsumePendingAuth(state)
	if !ok {
		return McpCallbackResult{HTML: errorPage("Invalid or expired authorization state. Please try again."), HTTPStatus: 400}
	}
	// 校验回调 user 与 pending auth user 一致。
	if userID != pending.UserID {
		return McpCallbackResult{HTML: errorPage("User mismatch. Please try again."), HTTPStatus: 403}
	}
	// 取/注册 client。
	client, err := w.svc.EnsureClient(ctx, pending.ServerOrigin, callbackURL)
	if err != nil {
		log.Printf("mcp-oauth: ensure client: %v", err)
		return McpCallbackResult{HTML: errorPage("Failed to load OAuth client"), HTTPStatus: 500}
	}
	// code 换 token。
	tokenResp, err := w.svc.ExchangeCode(ctx, client, code, pending.CodeVerifier, callbackURL)
	if err != nil {
		log.Printf("mcp-oauth: exchange code: %v", err)
		return McpCallbackResult{HTML: errorPage(fmt.Sprintf("Token exchange failed: %v", err)), HTTPStatus: 500}
	}
	// upsert token。
	if err := w.svc.UpsertToken(ctx, userID, pending.ServerOrigin, tokenResp); err != nil {
		log.Printf("mcp-oauth: upsert token: %v", err)
		return McpCallbackResult{HTML: errorPage("Failed to store token"), HTTPStatus: 500}
	}
	// 通知 workspace agent reload config（best-effort）。
	// MCP 连接变更后的下游 reload 不由 media_auth 主动通知——消费方轮询连接状态或订阅事件（spec §8）。
	return McpCallbackResult{HTML: successPage(pending.ServerOrigin), HTTPStatus: 200}
}

// ── popup HTML（对齐 control-plane successPage / errorPage） ──────────

func successPage(serverOrigin string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>Authorization Complete</title></head>
<body>
<p>Authorization successful. This window will close automatically.</p>
<script>
  if (window.opener) {
    window.opener.postMessage({ type: 'mcp-oauth-complete', server_origin: %q }, '*');
  }
  window.close();
</script>
</body></html>`, serverOrigin)
}

func errorPage(message string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>Authorization Failed</title></head>
<body>
<p>%s</p>
<p><button onclick="window.close()">Close</button></p>
</body></html>`, html.EscapeString(message))
}
