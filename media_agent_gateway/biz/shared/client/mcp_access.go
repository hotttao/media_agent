package client

import (
	"context"
	"errors"
	sdk "media_agent/hertz_gen/media_plugin/mcp_access_service"
	pb "media_agent/hertz_gen/model/media_plugin/mcp_access"
	"media_agent/hertz_infra/mcpupstream"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrMCPAccess = errors.New("fixed MCP binding access unavailable")

type MCPScope struct {
	Kind, ActorID, CredentialID                                                            string
	ExpiresAt                                                                              time.Time
	SessionID, UserID, WorkspaceID, AgentConfigID, AgentType, MetadataTargetContextVersion string
	OrganizationID                                                                         string
	ContextVersion                                                                         int64
}
type MCPGrant struct {
	DefinitionID                                                   string
	DefinitionVersion                                              int64
	AllowedMethods, AllowedTools, AllowedPrompts, AllowedResources []string
	ProxyAuthorization                                             string
	ProxyEndpoint                                                  string
	UpstreamHeaders                                                map[string]string
}
type MCPAccess struct {
	SnapshotVersion, AuthorizationRevision string
	Grants                                 []MCPGrant
	RestrictedDefinitionIDs                []string
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
	v, _, e := api.ResolveMCPAccess(ctx, &pb.AccessRequest{WorkspaceId: scope.WorkspaceID, AgentConfigId: scope.AgentConfigID, UserId: scope.UserID, ContextVersion: scope.ContextVersion, SessionId: scope.SessionID, TargetContextVersion: scope.MetadataTargetContextVersion})
	if e != nil || v == nil {
		return MCPAccess{}, ErrMCPAccess
	}
	if v.WorkspaceId != scope.WorkspaceID || v.AgentConfigId != scope.AgentConfigID || v.ContextVersion != scope.ContextVersion || v.SnapshotVersion == "" || v.AuthorizationRevision == "" || len(v.Grants) > 128 {
		return MCPAccess{}, ErrMCPAccess
	}
	if len(v.RestrictedDefinitionIds) > 128 {
		return MCPAccess{}, ErrMCPAccess
	}
	result := MCPAccess{SnapshotVersion: v.SnapshotVersion, AuthorizationRevision: v.AuthorizationRevision, RestrictedDefinitionIDs: v.RestrictedDefinitionIds}
	for _, grant := range v.Grants {
		if grant == nil || grant.DefinitionVersion < 1 || grant.DefinitionId == "platform" {
			return MCPAccess{}, ErrMCPAccess
		}
		value := MCPGrant{DefinitionID: grant.DefinitionId, DefinitionVersion: grant.DefinitionVersion, AllowedMethods: grant.AllowedMethods, AllowedTools: grant.AllowedTools, AllowedPrompts: grant.AllowedPrompts, AllowedResources: grant.AllowedResources, ProxyAuthorization: grant.ProxyAuthorization, UpstreamHeaders: map[string]string{}}
		if len(grant.UpstreamHeaders) > 1 {
			return MCPAccess{}, ErrMCPAccess
		}
		for key, secret := range grant.UpstreamHeaders {
			if (!strings.EqualFold(key, "Authorization") && !mcpupstream.ValidCredentialHeader(key)) || secret == "" || len(secret) > (16<<10)+7 || strings.ContainsAny(secret, "\r\n\x00") {
				return MCPAccess{}, ErrMCPAccess
			}
			value.UpstreamHeaders[http.CanonicalHeaderKey(key)] = secret
		}
		if grant.OauthProxy {
			if grant.BindingId == "" || grant.BindingVersion < 1 || len(value.UpstreamHeaders) > 0 || grant.ProxyAuthorization != "" {
				return MCPAccess{}, ErrMCPAccess
			}
			base, err := url.Parse(c.BaseURL)
			if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Scheme != "http" && base.Scheme != "https") {
				return MCPAccess{}, ErrMCPAccess
			}
			value.ProxyEndpoint = strings.TrimRight(c.BaseURL, "/") + "/internal/mcp/" + url.PathEscape(grant.BindingId) + "/" + strconv.FormatInt(grant.BindingVersion, 10)
			value.UpstreamHeaders["X-MCP-Gateway-Credential"] = c.Credential
		}
		result.Grants = append(result.Grants, value)
	}
	return result, nil
}
