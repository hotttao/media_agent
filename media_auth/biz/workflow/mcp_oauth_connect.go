// Package workflow — mcp_oauth_connect：MCP OAuth 回调编排。
//
// 消费 pendingAuth → 校验 user → 取/注册 client → code 换 token → upsert token →
// 通知 control_plane reload workspace config → 返回 popup HTML（postMessage + close）。
// 对齐 control-plane routes/mcp-oauth.ts callback。
package workflow

import (
	"context"
	"fmt"
	"html"
	"log"

	mcpoauthservice "media_agent/media_auth/biz/service/mcp_oauth"
)

// McpOAuthConnect 编排 MCP OAuth 回调。
type McpOAuthConnect struct {
	svc      *mcpoauthservice.Service
	notifier WorkspaceNotifier
}

// WorkspaceNotifier 通知 control_plane reload workspace config（MCP OAuth 回调成功后）。
//
// P1 默认 no-op（control_plane 集成在 P2，spec §8）；P2 注入真实 HTTP notifier。
type WorkspaceNotifier interface {
	// NotifyAgentReload 通知 workspace 重新加载指定配置项；best-effort，失败不阻断。
	NotifyAgentReload(ctx context.Context, workspaceID string, configKeys []string)
}

// NewMcpOAuthConnect 构造 MCP OAuth 回调 workflow。notifier 为 nil 时用 no-op。
func NewMcpOAuthConnect(svc *mcpoauthservice.Service, notifier WorkspaceNotifier) *McpOAuthConnect {
	if notifier == nil {
		notifier = noopNotifier{}
	}
	return &McpOAuthConnect{svc: svc, notifier: notifier}
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
	w.notifier.NotifyAgentReload(context.Background(), pending.WorkspaceID, []string{"config"})
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

// noopNotifier 是 P1 默认的 WorkspaceNotifier（control_plane 集成在 P2）。
type noopNotifier struct{}

func (noopNotifier) NotifyAgentReload(ctx context.Context, workspaceID string, configKeys []string) {
	log.Printf("mcp-oauth: noop notifyAgentReload(workspace=%s, keys=%v) — control_plane integration pending P2", workspaceID, configKeys)
}
