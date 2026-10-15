package mcp_oauth

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	domain "media_agent/media_auth/biz/domain/mcp_oauth"
	service "media_agent/media_auth/biz/service/mcp_oauth"
	"strings"
	"testing"
	"time"
)

type tokenRepo struct{ domain.McpRepo }

func (tokenRepo) GetToken(_ context.Context, user, origin string) (domain.McpToken, error) {
	if user != "user" || origin != "https://mcp.example.test" {
		return domain.McpToken{}, domain.ErrTokenNotFound
	}
	expiry := time.Now().Add(time.Hour)
	return domain.McpToken{AccessToken: "access-only", RefreshToken: "never-return-refresh", ExpiresAt: &expiry}, nil
}
func TestUsableTokenRequiresPluginAndDoesNotReturnRefreshSecret(t *testing.T) {
	t.Setenv("MEDIA_AUTH_MCP_PLUGIN_TOKEN", "plugin-secret")
	previous := McpOAuthSvc
	McpOAuthSvc = service.NewService(tokenRepo{}, nil, nil)
	t.Cleanup(func() { McpOAuthSvc = previous })
	h := server.New()
	h.POST("/internal/mcp-oauth/token", GetUsableToken)
	for _, tc := range []struct {
		name, credential, body string
		status                 int
		contains               string
	}{
		{"browser", "", `{"user_id":"user","server_origin":"https://mcp.example.test"}`, 403, "plugin_identity_required"},
		{"wrong service", "wrong", `{"user_id":"user","server_origin":"https://mcp.example.test"}`, 403, "plugin_identity_required"},
		{"wrong origin", "plugin-secret", `{"user_id":"user","server_origin":"https://mcp.example.test/path"}`, 400, "invalid_origin"},
		{"connected", "plugin-secret", `{"user_id":"user","server_origin":"https://mcp.example.test"}`, 200, "access-only"},
		{"no connection", "plugin-secret", `{"user_id":"other","server_origin":"https://mcp.example.test"}`, 200, "needs_reauth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := ut.PerformRequest(h.Engine, "POST", "/internal/mcp-oauth/token", &ut.Body{Body: strings.NewReader(tc.body), Len: len(tc.body)}, ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "X-MCP-Plugin-Credential", Value: tc.credential}).Result()
			if result.StatusCode() != tc.status || !strings.Contains(string(result.Body()), tc.contains) || strings.Contains(string(result.Body()), "never-return-refresh") {
				t.Fatalf("status=%d body=%s", result.StatusCode(), result.Body())
			}
			if string(result.Header.Peek("Cache-Control")) != "no-store" {
				t.Fatal("missing no-store")
			}

		})
	}
}
