package middleware

import (
	"context"
	"crypto/subtle"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	d "media_agent/media_plugin/biz/domain/plugin_binding"
)

type Authenticator interface {
	Authenticate(context.Context, string, string) (string, error)
}

var identity Authenticator
var controlCredential, gatewayCredential string

type userKey struct{}

func Configure(auth Authenticator, control, gateway string) {
	identity = auth
	controlCredential = control
	gatewayCredential = gateway
}
func User(ctx context.Context) string { value, _ := ctx.Value(userKey{}).(string); return value }
func matches(provided, expected string) bool {
	return expected != "" && provided != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}
func Session() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if identity == nil {
			c.JSON(503, map[string]string{"error": "identity_unavailable"})
			c.Abort()
			return
		}
		user, err := identity.Authenticate(ctx, string(c.GetHeader("Authorization")), string(c.GetHeader("Cookie")))
		if err != nil || user == "" {
			status := 503
			if errors.Is(err, d.ErrForbidden) || err == nil {
				status = 401
			}
			c.JSON(status, map[string]string{"error": "identity_required"})
			c.Abort()
			return
		}
		c.Next(context.WithValue(ctx, userKey{}, user))
	}
}
func Control() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if !matches(string(c.GetHeader("X-MCP-Control-Credential")), controlCredential) || len(c.GetHeader("X-MCP-Acting-User-ID")) == 0 {
			c.JSON(403, map[string]string{"error": "control_identity_required"})
			c.Abort()
			return
		}
		c.Next(context.WithValue(ctx, userKey{}, string(c.GetHeader("X-MCP-Acting-User-ID"))))
	}
}
func Gateway() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if !matches(string(c.GetHeader("X-MCP-Gateway-Credential")), gatewayCredential) {
			c.JSON(403, map[string]string{"error": "gateway_identity_required"})
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}
