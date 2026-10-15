package mcp_catalog

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"media_agent/hertz_infra/mcpprotocol"
	"net/http"
)

func serveProtocol(ctx context.Context, c *app.RequestContext, handler http.Handler) {
	mcpprotocol.ServeHertz(ctx, c, handler)
}
