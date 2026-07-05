// oauth_provider_service_test.go：oauth handler 错误→HTTP 响应映射测试（ut.CreateUtRequestContext）。
//
// 覆盖 writeOAuthError：*OAuthError → {error, error_description} + 自定 status；
// 非 OAuthError → 500 server_error。
package oauth

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	oauthservice "media_agent/media_auth/biz/service/oauth"
)

func TestWriteOAuthError_Mapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantDesc   string // 空表示不期望 description
	}{
		{"invalid_client 400", oauthservice.NewOAuthError("invalid_client", "", 400), 400, "invalid_client", ""},
		{"invalid_grant 400 + desc", oauthservice.NewOAuthError("invalid_grant", "code expired", 400), 400, "invalid_grant", "code expired"},
		{"invalid_client 401", oauthservice.NewOAuthError("invalid_client", "bad secret", 401), 401, "invalid_client", "bad secret"},
		{"unsupported_grant_type 400", oauthservice.NewOAuthError("unsupported_grant_type", "", 400), 400, "unsupported_grant_type", ""},
		{"invalid_token 401", oauthservice.NewOAuthError("invalid_token", "", 401), 401, "invalid_token", ""},
		{"non-OAuthError → 500 server_error", errors.New("db down"), 500, "server_error", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := ut.CreateUtRequestContext("POST", "/api/oauth/token", nil)
			writeOAuthError(ctx, c.err)
			if ctx.Response.StatusCode() != c.wantStatus {
				t.Fatalf("status: got %d, want %d", ctx.Response.StatusCode(), c.wantStatus)
			}
			var body map[string]string
			if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
				t.Fatalf("unmarshal body: %v (body=%s)", err, string(ctx.Response.Body()))
			}
			if body["error"] != c.wantCode {
				t.Errorf("error code: got %q, want %q", body["error"], c.wantCode)
			}
			if c.wantDesc != "" && body["error_description"] != c.wantDesc {
				t.Errorf("error_description: got %q, want %q", body["error_description"], c.wantDesc)
			}
		})
	}
}

// OAuthError 不带 description 时，响应不含 error_description 字段。
func TestWriteOAuthError_NoDescriptionOmitted(t *testing.T) {
	ctx := ut.CreateUtRequestContext("POST", "/api/oauth/token", nil)
	writeOAuthError(ctx, oauthservice.NewOAuthError("invalid_client", "", 400))
	var body map[string]string
	_ = json.Unmarshal(ctx.Response.Body(), &body)
	if _, present := body["error_description"]; present {
		t.Errorf("empty description should be omitted, body=%+v", body)
	}
}
