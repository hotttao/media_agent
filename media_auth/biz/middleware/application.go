package middleware

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"strings"
)

// ApplicationProtocol skips only the dedicated protocol routes. Every handler verifies
// its own Basic or application Bearer credential. Unknown paths still use normal auth.
func ApplicationProtocol(session app.HandlerFunc) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		path := string(c.Request.Path())
		method := string(c.Request.Method())
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		dedicated := method == "GET" && path == "/apps/v1/me" || method == "POST" && (path == "/internal/apps/verify" || path == "/internal/apps/caller")
		if len(parts) >= 4 && parts[0] == "apps" && parts[1] == "v1" && parts[2] == "users" && parts[3] != "" {
			dedicated = dedicated || (len(parts) == 4 && (method == "GET" || method == "PUT")) || (len(parts) == 5 && parts[4] == "token" && method == "POST")
		}
		if dedicated {
			c.Next(ctx)
			return
		}
		if session == nil {
			abortUnauthorized(c, "missing session verifier")
			return
		}
		session(ctx, c)
	}
}
