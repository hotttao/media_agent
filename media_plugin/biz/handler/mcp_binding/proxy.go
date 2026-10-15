package mcp_binding

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	"media_agent/hertz_infra/mcpprotocol"
	"media_agent/hertz_infra/mcpupstream"
	d "media_agent/media_plugin/biz/domain/plugin_binding"
	"media_agent/media_plugin/biz/workflow"
	"net/http"
	"slices"
	"strconv"
)

type proxyCaller struct {
	scope   workflow.Scope
	binding string
	version int64
}

func (p proxyCaller) Call(ctx context.Context, method string, params []byte) ([]byte, bool, error) {
	if !slices.Contains([]string{"tools/list", "tools/call", "prompts/list", "prompts/get", "resources/list", "resources/templates/list", "resources/read"}, method) {
		return nil, false, nil
	}
	definition, headers, err := application.OAuthBackend(ctx, p.scope, p.binding, p.version)
	if err != nil {
		return nil, true, err
	}
	peer, err := (mcpupstream.MCPUpstream{}).Open(ctx, mcpupstream.MCPBackend{Endpoint: definition.Endpoint, Transport: definition.Transport, Headers: headers})
	if err != nil {
		return nil, true, err
	}
	defer peer.Close()
	raw, err := peer.Call(ctx, method, params)
	return raw, true, err
}
func ProxyMCP(ctx context.Context, c *app.RequestContext) {
	if !ready(c) {
		return
	}
	version, err := strconv.ParseInt(c.Param("binding_version"), 10, 64)
	if err != nil || version < 1 {
		failure(c, d.ErrInvalid)
		return
	}
	contextVersion, err := strconv.ParseInt(string(c.GetHeader("X-Media-Context-Version")), 10, 64)
	if err != nil {
		failure(c, d.ErrInvalid)
		return
	}
	scope := workflow.Scope{UserID: string(c.GetHeader("X-Media-User-ID")), WorkspaceID: string(c.GetHeader("X-Media-Workspace-ID")), AgentConfigID: string(c.GetHeader("X-Media-Agent-Configuration-ID")), SessionID: string(c.GetHeader("X-Media-Session-ID")), Target: string(c.GetHeader("X-Media-Metadata-Target")), ContextVersion: contextVersion}
	id := c.Param("binding_id")
	_, _, err = application.OAuthBackend(ctx, scope, id, version)
	if errors.Is(err, d.ErrReauth) {
		c.JSON(401, map[string]string{"error": "needs_reauth"})
		return
	}
	if err != nil {
		failure(c, err)
		return
	}
	handler := mcpprotocol.NewHandler(func(context.Context, http.Header) (mcpprotocol.Caller, int, error) {
		return proxyCaller{scope, id, version}, 200, nil
	})
	mcpprotocol.ServeHertz(ctx, c, handler)
}
