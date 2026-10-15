package client

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/cloudwego/hertz/pkg/protocol"
	sdk "media_agent/hertz_gen/media_plugin/mcp_binding_service"
	pb "media_agent/hertz_gen/model/media_plugin/mcp_binding"
	d "media_agent/media_control/biz/domain/agent_configuration"
	"net/http"
	"time"
)

type MCPBindings struct{ BaseURL, Credential string }

func (c MCPBindings) SelectedBinding(ctx context.Context, user, workspace, id string, version int64) (*d.Resource, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if c.BaseURL == "" || c.Credential == "" {
		return nil, errors.New("MCP binding authority unavailable")
	}
	api, err := sdk.NewMcpBindingServiceClient(c.BaseURL, sdk.WithResponseResultDecider(func(int, *protocol.Response) bool { return false }), sdk.WithHeader(http.Header{"X-MCP-Control-Credential": {c.Credential}, "X-MCP-Acting-User-ID": {user}}))
	if err != nil {
		return nil, errors.New("MCP binding authority unavailable")
	}
	value, resp, err := api.GetSelectedBinding(ctx, &pb.BindingQuery{WorkspaceId: workspace, BindingId: id, Version: version})
	if resp != nil {
		switch resp.StatusCode() {
		case 401, 403:
			return nil, d.ErrForbidden
		case 400, 404:
			return nil, d.ErrInvalid
		case 409:
			return nil, d.ErrConflict
		}
	}
	if err != nil || value == nil || value.BindingId != id || value.WorkspaceId != workspace || value.Version != version || !value.Enabled || value.DefinitionId == "platform" {
		return nil, errors.New("MCP binding authority unavailable")
	}
	hash, err := hex.DecodeString(value.Checksum)
	if err != nil || len(hash) != 32 {
		return nil, errors.New("invalid MCP binding checksum")
	}
	return &d.Resource{ResourceReference: d.ResourceReference{Kind: "media_plugin.mcp_binding", ID: id, Version: version, Checksum: value.Checksum}, Name: value.DefinitionId}, nil
}
