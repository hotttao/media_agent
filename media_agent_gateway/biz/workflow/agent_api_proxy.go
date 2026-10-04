package workflow

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"strings"

	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/media_agent_gateway/biz/shared/client"
)

var (
	ErrAgentProxyInvalid     = errors.New("invalid Agent proxy request")
	ErrAgentProxyForbidden   = errors.New("Agent delegation forbidden")
	ErrAgentProxyUnavailable = errors.New("Agent proxy unavailable")
)

type AgentProxyRequest = client.AgentProxyRequest
type AgentProxyResponse = client.AgentProxyResponse
type AgentCredentialVerifier interface {
	VerifyAgentCredential(context.Context, string) (internaljwt.AgentPrincipal, error)
}
type AgentRequestSigner interface {
	Sign(internaljwt.AgentPrincipal, string, internaljwt.AgentRequest) (string, error)
}
type AgentProxyForwarder interface {
	ForwardAgentRequest(context.Context, AgentProxyRequest, string) (AgentProxyResponse, error)
}
type AgentAPIProxy struct {
	credentials AgentCredentialVerifier
	authority   AgentTokenAuthority
	signer      AgentRequestSigner
	forwarder   AgentProxyForwarder
}

func NewAgentAPIProxy(credentials AgentCredentialVerifier, authority AgentTokenAuthority, signer AgentRequestSigner, forwarder AgentProxyForwarder) *AgentAPIProxy {
	return &AgentAPIProxy{credentials: credentials, authority: authority, signer: signer, forwarder: forwarder}
}

func (w *AgentAPIProxy) Execute(ctx context.Context, credential string, request AgentProxyRequest) (AgentProxyResponse, error) {
	audience, ok := agentProxyAudience(request)
	if !ok {
		return AgentProxyResponse{}, ErrAgentProxyInvalid
	}
	if w == nil || w.credentials == nil || w.authority == nil || w.signer == nil || w.forwarder == nil {
		return AgentProxyResponse{}, ErrAgentProxyUnavailable
	}
	p, err := w.credentials.VerifyAgentCredential(ctx, credential)
	if err != nil {
		return AgentProxyResponse{}, err
	}
	configuration, ok := strings.CutPrefix(p.ActorID, "Agent:")
	if !ok || !validConfigurationID(configuration) || p.UserID == "" || p.OrganizationID == "" || p.CredentialID == "" {
		return AgentProxyResponse{}, ErrAgentProxyForbidden
	}
	facts, err := w.authority.ResolveAgentIdentity(ctx, configuration)
	if err != nil {
		return AgentProxyResponse{}, err
	}
	if facts.ConfigurationID != configuration || facts.ActorID != p.ActorID || facts.OwnerUserID != p.UserID || facts.OrganizationID != p.OrganizationID || facts.WorkspaceID == "" || facts.TeamID == "" || !facts.OwnerAuthorized || (facts.State != "running" && facts.State != "stopped") {
		return AgentProxyResponse{}, ErrAgentProxyForbidden
	}
	// Workspace is authoritative Control state, never a client-supplied claim.
	p.WorkspaceID = facts.WorkspaceID
	assertion, err := w.signer.Sign(p, audience, request.Request)
	if err != nil {
		return AgentProxyResponse{}, ErrAgentProxyUnavailable
	}
	return w.forwarder.ForwardAgentRequest(internaljwt.ContextWithAgentPrincipal(ctx, p), request, assertion)
}

// Match explicit business routes, never an arbitrary URL or service prefix.
func agentProxyAudience(r AgentProxyRequest) (string, bool) {
	if len(r.Request.Body) > 8<<20 || len(r.Request.URI) > 4096 || len(r.Request.ContentType) > 1024 || strings.ContainsAny(r.Request.URI+r.Request.ContentType, "\r\n\x00\\#") {
		return "", false
	}
	u, err := url.ParseRequestURI(r.Request.URI)
	if err != nil || u.IsAbs() || u.Host != "" || u.RawPath != "" || strings.Contains(u.Path, "%") || !strings.HasPrefix(u.Path, "/api/v1/") {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/api/v1/"), "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return "", false
		}
	}
	if r.Request.ContentType != "" {
		kind, params, err := mime.ParseMediaType(r.Request.ContentType)
		if err != nil || (kind != "application/json" && kind != "application/octet-stream" && kind != "multipart/form-data") {
			return "", false
		}
		if kind == "multipart/form-data" {
			writer := multipart.NewWriter(io.Discard)
			if err := writer.SetBoundary(params["boundary"]); err != nil {
				return "", false
			}
		}
	}
	m := r.Request.Method
	id := len(parts) >= 2 && validConfigurationID(parts[1])
	crud := (len(parts) == 1 && (m == "GET" || m == "POST")) || (len(parts) == 2 && id && (m == "GET" || m == "PUT" || m == "DELETE"))
	switch r.Service {
	case "materials":
		allowed := false
		switch parts[0] {
		case "materials":
			allowed = crud
		case "products", "virtual-ips":
			allowed = crud || (len(parts) == 3 && id && parts[2] == "archive" && m == "POST")
		case "generated-assets":
			allowed = m == "GET" && (len(parts) == 1 || len(parts) == 2 && id) || m == "DELETE" && len(parts) == 2 && id && parts[1] != "by-source"
		case "asset-kinds":
			allowed = m == "GET" && (len(parts) == 1 || len(parts) == 2 && id)
		case "library-objects:upload":
			allowed = len(parts) == 1 && m == "POST"
		case "library-objects:resolve":
			allowed = len(parts) == 1 && m == "GET"
		}
		return "media-materials-agent", allowed
	case "workflow":
		allowed := false
		switch parts[0] {
		case "workflows", "templates":
			allowed = (len(parts) == 1 && (m == "GET" || m == "POST")) || (len(parts) == 2 && id && (m == "GET" || m == "PUT"))
			if len(parts) == 3 && id && m == "POST" {
				switch parts[2] {
				case "archive", "publish", "grants", "visibility":
					allowed = true
				}
			}
			if len(parts) == 4 && id && parts[2] == "grants" && validConfigurationID(parts[3]) && m == "DELETE" {
				allowed = true
			}
			if parts[0] == "workflows" && len(parts) == 2 && parts[1] == "validate" {
				allowed = m == "POST"
			}
			if parts[0] == "templates" && len(parts) == 4 && id && parts[2] == "versions" && validConfigurationID(parts[3]) && m == "GET" {
				allowed = true
			}
		case "jobs":
			allowed = len(parts) == 1 && (m == "GET" || m == "POST") || len(parts) == 2 && id && m == "GET" || len(parts) == 3 && id && m == "POST" && (parts[2] == "cancel" || parts[2] == "retry" || parts[2] == "resume")
		case "step-runs":
			allowed = id && parts[1] != "claim" && (len(parts) == 2 && m == "GET" || len(parts) == 3 && m == "POST" && (parts[2] == "confirm" || parts[2] == "regenerate" || parts[2] == "retry"))
		case "historical-video-pushes":
			allowed = m == "GET" && (len(parts) == 1 || len(parts) == 2 && id)
		}
		return "media-workflow-agent", allowed
	}
	return "", false
}
