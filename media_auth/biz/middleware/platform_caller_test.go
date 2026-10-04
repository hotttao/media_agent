package middleware

import (
	"context"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestPlatformCallerRejectsPlaintextAndForwardedClaims(t *testing.T) {
	c := &app.RequestContext{}
	c.Request.Header.Set("X-Caller-Service", "media_agent_gateway")
	c.Request.Header.Set("X-Forwarded-Client-Cert", "spiffe://media-agent/services/media_agent_gateway")
	c.Request.Header.Set("Authorization", "Bearer test-internal-credential")
	PlatformCaller(nil)(context.Background(), c)
	if !c.IsAborted() || c.Response.StatusCode() != 403 {
		t.Fatal("plaintext or forwarded claims admitted as platform caller")
	}
}
