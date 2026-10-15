package middleware

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	client "media_agent/media_agent_gateway/biz/shared/client"
	"strings"
	"testing"
)

func TestMatchesInternalToken(t *testing.T) {
	if !matchesInternalToken("service-token", "service-token") {
		t.Fatal("matching internal token was rejected")
	}
	for _, value := range [][2]string{{"", "service-token"}, {"service-token", ""}, {"wrong", "service-token"}} {
		if matchesInternalToken(value[0], value[1]) {
			t.Fatalf("invalid token accepted: %q %q", value[0], value[1])
		}
	}
}

type browserIdentity struct{}

func (browserIdentity) Authenticate(_ context.Context, authorization, cookie string) (client.GatewayUser, error) {
	if authorization == "Bearer edge" && cookie == "ory_kratos_session=verified" {
		return client.GatewayUser{UserID: "current-user"}, nil
	}
	if authorization == "Bearer unavailable" {
		return client.GatewayUser{}, client.ErrAgentTokenUnavailable
	}
	return client.GatewayUser{}, client.ErrAgentTokenUnauthorized
}
func TestBrowserSessionUsesCurrentIdentityAndDoesNotFallbackAfterDenial(t *testing.T) {
	previous := managementAuthenticator
	ConfigureAgentTokenManagement(browserIdentity{})
	t.Cleanup(func() { managementAuthenticator = previous })
	h := server.New()
	h.GET("/protected", Session(), func(ctx context.Context, c *app.RequestContext) {
		c.JSON(200, map[string]string{"user_id": UserID(ctx)})
	})
	for _, tc := range []struct {
		authorization, cookie string
		status                int
	}{
		{"Bearer edge", "ory_kratos_session=verified", 200},
		{"Bearer denied", "token=old-cookie", 401},
		{"Bearer unavailable", "token=old-cookie", 503},
		{"", "ory_kratos_session=verified", 401},
	} {
		response := ut.PerformRequest(h.Engine, "GET", "/protected", nil, ut.Header{Key: "Authorization", Value: tc.authorization}, ut.Header{Key: "Cookie", Value: tc.cookie}).Result()
		if response.StatusCode() != tc.status {
			t.Fatalf("status=%d expected=%d", response.StatusCode(), tc.status)
		}
		if tc.status == 200 && !strings.Contains(string(response.Body()), "current-user") {
			t.Fatal("current identity was not propagated")
		}
	}
}
