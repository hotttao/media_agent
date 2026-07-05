// mcp_oauth_connect_test.go：MCP OAuth 回调 workflow 编排测试（fake service）。
//
// 覆盖：error 参数 → errorPage；缺 code/state → 400；pendingAuth 无效 → 400；
//
//	user 不匹配 → 403；EnsureClient 失败 → 500；ExchangeCode 失败 → 500；
//	UpsertToken 失败 → 500；成功 → successPage（含 server_origin）。
package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	domainmcp "media_agent/media_auth/biz/domain/mcp_oauth"
	"media_agent/media_auth/biz/shared/pendingauth"
)

// fakeMcpSvc 实现 McpOAuthService 接缝。
type fakeMcpSvc struct {
	pending      pendingauth.PendingAuth
	pendingOK    bool
	client       domainmcp.McpClient
	ensureErr    error
	tokenResp    domainmcp.TokenResponse
	exchangeErr  error
	upsertErr    error
	upsertCalled bool
}

func (f *fakeMcpSvc) ConsumePendingAuth(_ string) (pendingauth.PendingAuth, bool) {
	return f.pending, f.pendingOK
}
func (f *fakeMcpSvc) EnsureClient(_ context.Context, _, _ string) (domainmcp.McpClient, error) {
	return f.client, f.ensureErr
}
func (f *fakeMcpSvc) ExchangeCode(_ context.Context, _ domainmcp.McpClient, _, _, _ string) (domainmcp.TokenResponse, error) {
	return f.tokenResp, f.exchangeErr
}
func (f *fakeMcpSvc) UpsertToken(_ context.Context, _, _ string, _ domainmcp.TokenResponse) error {
	f.upsertCalled = true
	return f.upsertErr
}

func newMcpFlow(svc McpOAuthService) *McpOAuthConnect {
	return &McpOAuthConnect{svc: svc}
}

// --- 错误路径 ---

func TestMcpCallback_ErrorParam(t *testing.T) {
	w := newMcpFlow(&fakeMcpSvc{})
	res := w.HandleCallback(context.Background(), "u1", "", "", "access_denied", "https://cb")
	if res.HTTPStatus != 200 || !strings.Contains(res.HTML, "Authorization denied: access_denied") {
		t.Errorf("error param: got res=%+v", res)
	}
}

func TestMcpCallback_MissingCodeOrState(t *testing.T) {
	w := newMcpFlow(&fakeMcpSvc{})
	res := w.HandleCallback(context.Background(), "u1", "", "state", "", "https://cb")
	if res.HTTPStatus != 400 || !strings.Contains(res.HTML, "Missing code or state") {
		t.Errorf("missing code: got res=%+v", res)
	}
}

func TestMcpCallback_InvalidState(t *testing.T) {
	w := newMcpFlow(&fakeMcpSvc{pendingOK: false})
	res := w.HandleCallback(context.Background(), "u1", "code", "bad-state", "", "https://cb")
	if res.HTTPStatus != 400 || !strings.Contains(res.HTML, "Invalid or expired") {
		t.Errorf("invalid state: got res=%+v", res)
	}
}

func TestMcpCallback_UserMismatch(t *testing.T) {
	w := newMcpFlow(&fakeMcpSvc{
		pending:   pendingauth.PendingAuth{UserID: "u_other", ServerOrigin: "https://mcp", CodeVerifier: "v"},
		pendingOK: true,
	})
	res := w.HandleCallback(context.Background(), "u1", "code", "state", "", "https://cb")
	if res.HTTPStatus != 403 || !strings.Contains(res.HTML, "User mismatch") {
		t.Errorf("user mismatch: got res=%+v", res)
	}
}

func TestMcpCallback_EnsureClientFail(t *testing.T) {
	w := newMcpFlow(&fakeMcpSvc{
		pending:   pendingauth.PendingAuth{UserID: "u1", ServerOrigin: "https://mcp", CodeVerifier: "v"},
		pendingOK: true,
		ensureErr: errors.New("discover failed"),
	})
	res := w.HandleCallback(context.Background(), "u1", "code", "state", "", "https://cb")
	if res.HTTPStatus != 500 || !strings.Contains(res.HTML, "Failed to load OAuth client") {
		t.Errorf("ensure fail: got res=%+v", res)
	}
}

func TestMcpCallback_ExchangeFail(t *testing.T) {
	w := newMcpFlow(&fakeMcpSvc{
		pending:     pendingauth.PendingAuth{UserID: "u1", ServerOrigin: "https://mcp", CodeVerifier: "v"},
		pendingOK:   true,
		client:      domainmcp.McpClient{ServerOrigin: "https://mcp"},
		exchangeErr: errors.New("token endpoint 400"),
	})
	res := w.HandleCallback(context.Background(), "u1", "code", "state", "", "https://cb")
	if res.HTTPStatus != 500 || !strings.Contains(res.HTML, "Token exchange failed") {
		t.Errorf("exchange fail: got res=%+v", res)
	}
}

func TestMcpCallback_UpsertFail(t *testing.T) {
	w := newMcpFlow(&fakeMcpSvc{
		pending:   pendingauth.PendingAuth{UserID: "u1", ServerOrigin: "https://mcp", CodeVerifier: "v"},
		pendingOK: true,
		client:    domainmcp.McpClient{ServerOrigin: "https://mcp"},
		tokenResp: domainmcp.TokenResponse{AccessToken: "at"},
		upsertErr: errors.New("db down"),
	})
	res := w.HandleCallback(context.Background(), "u1", "code", "state", "", "https://cb")
	if res.HTTPStatus != 500 || !strings.Contains(res.HTML, "Failed to store token") {
		t.Errorf("upsert fail: got res=%+v", res)
	}
}

// --- 成功路径 ---

func TestMcpCallback_Success(t *testing.T) {
	svc := &fakeMcpSvc{
		pending:   pendingauth.PendingAuth{UserID: "u1", ServerOrigin: "https://mcp.example.com", CodeVerifier: "v"},
		pendingOK: true,
		client:    domainmcp.McpClient{ServerOrigin: "https://mcp.example.com"},
		tokenResp: domainmcp.TokenResponse{AccessToken: "at", RefreshToken: "rt"},
	}
	w := newMcpFlow(svc)
	res := w.HandleCallback(context.Background(), "u1", "code", "state", "", "https://cb")
	if res.HTTPStatus != 200 || !strings.Contains(res.HTML, "Authorization successful") {
		t.Errorf("success: got res=%+v", res)
	}
	// successPage 含 server_origin（postMessage 用）。
	if !strings.Contains(res.HTML, "mcp.example.com") {
		t.Errorf("success page missing server_origin: %s", res.HTML)
	}
	// upsert 被调。
	if !svc.upsertCalled {
		t.Errorf("UpsertToken not called on success")
	}
}
