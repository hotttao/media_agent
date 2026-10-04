package middleware

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/network"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"media_agent/media_auth/biz/shared/platformmtls"
)

// PlatformCaller is exclusive to the dedicated mTLS listener. A generated
// route accidentally registered on plaintext/public HTTP still fails closed.
// It never reads a header, body, session or delegation JWT for service identity.
func PlatformCaller(verifier *platformmtls.Verifier) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")
		conn, ok := c.GetConn().(network.ConnTLSer)
		if !ok || verifier == nil {
			c.AbortWithStatusJSON(consts.StatusForbidden, map[string]string{"error": "platform caller authentication failed"})
			return
		}
		state := conn.ConnectionState()
		bound, err := verifier.Bind(ctx, &state)
		if err != nil {
			c.AbortWithStatusJSON(consts.StatusForbidden, map[string]string{"error": "platform caller authentication failed"})
			return
		}
		c.Next(bound)
	}
}
