package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"media_agent/hertz_infra/authn/mcpdelegation"
	"media_agent/media_agent_gateway/biz/shared/client"
)

type MCPModuleAccess interface {
	ModuleAllowed(context.Context, MCPContext, string) (bool, error)
}
type MCPBusiness struct {
	DefinitionID string
	Access       MCPModuleAccess
	Signer       *mcpdelegation.Signer
}

func (w *MCPGateway) WithBusiness(b *MCPBusiness) *MCPGateway { w.business = b; return w }
func (w *MCPGateway) businessSource(ctx context.Context, scope MCPContext) (*mcpSource, error) {
	b := w.business
	if b == nil {
		return nil, nil
	}
	if b.Access == nil || b.Signer == nil {
		return nil, ErrMCPUnavailable
	}
	permitted, err := b.Access.ModuleAllowed(ctx, scope, "workflow")
	if errors.Is(err, client.ErrMCPUserDenied) {
		return nil, nil
	}
	if err != nil {
		return nil, ErrMCPUnavailable
	}
	if !permitted {
		return nil, nil
	}
	definition, err := w.catalog.Get(ctx, b.DefinitionID)
	if err != nil {
		return nil, ErrMCPUnavailable
	}
	if definition.Status != "enabled" {
		return nil, nil
	}
	if definition.OrganizationID != "" || definition.Transport != "streamable_http" || definition.AuthenticationType != "session_context" {
		return nil, ErrMCPUnavailable
	}
	source := &mcpSource{definitionID: definition.ID, version: definition.Version, currentVersion: definition.Version, grant: client.MCPGrant{DefinitionID: definition.ID, DefinitionVersion: definition.Version, AllowedMethods: []string{"tools/list", "tools/call"}, AllowedTools: []string{"*"}}, backend: client.MCPBackend{Endpoint: definition.Endpoint, Transport: definition.Transport}}
	source.backend.RequestHeaders = func(callCtx context.Context, method string, params json.RawMessage) (map[string]string, error) {
		// The transport invokes this separately for handshake, discovery and call;
		// an initialization assertion can never authorize a business invocation.
		callID, err := mcpdelegation.NewCallID()
		if err != nil {
			return nil, err
		}
		p := mcpdelegation.Principal{Kind: scope.Kind, UserID: scope.UserID, ActorID: scope.ActorID, CredentialID: scope.CredentialID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID}
		g := mcpdelegation.Grant{Principal: p, Audience: mcpdelegation.Backend, CallID: callID, SourceID: definition.ID, Method: method, ExpiresAt: scope.ExpiresAt}
		if deadline, ok := callCtx.Deadline(); ok && (g.ExpiresAt.IsZero() || deadline.Before(g.ExpiresAt)) {
			g.ExpiresAt = deadline
		}
		if g.ExpiresAt.IsZero() {
			g.ExpiresAt = time.Now().Add(mcpdelegation.MaxLifetime)
		}
		if method == "tools/call" {
			var args struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if json.Unmarshal(params, &args) != nil {
				return nil, ErrMCPArgument
			}
			g.Operation = args.Name
			g.ArgumentsDigest, err = mcpdelegation.ArgumentsDigest(args.Arguments)
			if err != nil {
				return nil, ErrMCPArgument
			}
		}
		assertion, err := b.Signer.Sign(g)
		if err != nil {
			return nil, ErrMCPUnavailable
		}
		headers := map[string]string{"Authorization": "Bearer " + assertion}
		if method != "tools/call" {
			return headers, nil
		}
		g.Method = ""
		g.ArgumentsDigest = ""
		g.Presenter = mcpdelegation.Backend
		g.Audience = mcpdelegation.Workflow
		headers[mcpdelegation.WorkflowHeader], err = b.Signer.Sign(g)
		if err != nil {
			return nil, ErrMCPUnavailable
		}
		if g.Operation == "jobs_submit" || g.Operation == "jobs_get" {
			permitted, err := b.Access.ModuleAllowed(callCtx, scope, "materials")
			if err != nil && !errors.Is(err, client.ErrMCPUserDenied) {
				return nil, ErrMCPUnavailable
			}
			if permitted {
				g.Audience = mcpdelegation.Materials
				headers[mcpdelegation.MaterialsHeader], err = b.Signer.Sign(g)
				if err != nil {
					return nil, ErrMCPUnavailable
				}
			}
		}
		return headers, nil
	}
	return source, nil
}
func scopeIdentity(scope MCPContext) string {
	raw, _ := json.Marshal([]any{scope.Kind, scope.UserID, scope.ActorID, scope.CredentialID, scope.OrganizationID, scope.WorkspaceID, scope.SessionID, scope.AgentConfigID, scope.ContextVersion, scope.MetadataTargetContextVersion})
	return string(raw)
}
