package client

import (
	"context"
	"errors"
	sdk "media_agent/hertz_gen/media_plugin/mcp_access_service"
	pb "media_agent/hertz_gen/model/media_plugin/mcp_access"
	"net/http"
	"time"
)

var ErrMCPAccess = errors.New("fixed MCP binding access unavailable")

type MCPScope struct {
	SessionID, UserID, WorkspaceID, AgentConfigID, AgentType, MetadataTargetContextVersion string
	ContextVersion                                                                         int64
}
type MCPGrant struct {
	DefinitionID                                                   string
	DefinitionVersion                                              int64
	AllowedMethods, AllowedTools, AllowedPrompts, AllowedResources []string
	ProxyAuthorization                                             string
}
type MCPAccess struct {
	SnapshotVersion, AuthorizationRevision string
	Grants                                 []MCPGrant
}
type PluginMCPAccess struct{ BaseURL, Credential string }

func (c PluginMCPAccess) Resolve(ctx context.Context, scope MCPScope) (MCPAccess, error) {
	if c.BaseURL == "" || c.Credential == "" {
		return MCPAccess{}, ErrMCPAccess
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	api, e := sdk.NewMcpAccessServiceClient(c.BaseURL, sdk.WithHeader(http.Header{"X-MCP-Gateway-Credential": {c.Credential}}))
	if e != nil {
		return MCPAccess{}, ErrMCPAccess
	}
	v, _, e := api.ResolveMCPAccess(ctx, &pb.AccessRequest{WorkspaceId: scope.WorkspaceID, AgentConfigId: scope.AgentConfigID, UserId: scope.UserID, ContextVersion: scope.ContextVersion, SessionId: scope.SessionID})
	if e != nil || v == nil {
		return MCPAccess{}, ErrMCPAccess
	}
	if v.WorkspaceId != scope.WorkspaceID || v.AgentConfigId != scope.AgentConfigID || v.ContextVersion != scope.ContextVersion || v.SnapshotVersion == "" || v.AuthorizationRevision == "" || len(v.Grants) > 128 {
		return MCPAccess{}, ErrMCPAccess
	}
	result := MCPAccess{SnapshotVersion: v.SnapshotVersion, AuthorizationRevision: v.AuthorizationRevision}
	for _, grant := range v.Grants {
		if grant == nil || grant.DefinitionVersion < 1 || grant.DefinitionId == "platform" {
			return MCPAccess{}, ErrMCPAccess
		}
		result.Grants = append(result.Grants, MCPGrant{DefinitionID: grant.DefinitionId, DefinitionVersion: grant.DefinitionVersion, AllowedMethods: grant.AllowedMethods, AllowedTools: grant.AllowedTools, AllowedPrompts: grant.AllowedPrompts, AllowedResources: grant.AllowedResources, ProxyAuthorization: grant.ProxyAuthorization})
	}
	return result, nil
}
