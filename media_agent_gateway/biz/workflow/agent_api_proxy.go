package workflow

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"strings"
	"time"

	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/delegationaudit"
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
	audit       delegationaudit.Recorder
}

func NewAgentAPIProxy(credentials AgentCredentialVerifier, authority AgentTokenAuthority, signer AgentRequestSigner, forwarder AgentProxyForwarder) *AgentAPIProxy {
	return NewAgentAPIProxyWithAudit(credentials, authority, signer, forwarder, delegationaudit.LogRecorder{})
}

func NewAgentAPIProxyWithAudit(credentials AgentCredentialVerifier, authority AgentTokenAuthority, signer AgentRequestSigner, forwarder AgentProxyForwarder, audit delegationaudit.Recorder) *AgentAPIProxy {
	if audit == nil {
		audit = delegationaudit.LogRecorder{}
	}
	return &AgentAPIProxy{credentials: credentials, authority: authority, signer: signer, forwarder: forwarder, audit: audit}
}

func (w *AgentAPIProxy) Execute(ctx context.Context, credential string, request AgentProxyRequest) (response AgentProxyResponse, resultErr error) {
	started := time.Now()
	event := delegationaudit.Event{Event: "agent_proxy_request", Phase: "gateway", Stage: "request_validation", Outcome: "invalid_request", StatusCode: 400}
	defer func() {
		panicValue := recover()
		if panicValue != nil {
			event.StatusCode, event.Outcome = 500, "panic"
		}
		event.DurationMS = time.Since(started).Milliseconds()
		recorder := delegationaudit.Recorder(delegationaudit.LogRecorder{})
		if w != nil && w.audit != nil {
			recorder = w.audit
		}
		recorder.Record(ctx, event)
		if panicValue != nil {
			panic(panicValue)
		}
	}()
	audience, ok := agentProxyAudience(request)
	if !ok {
		return AgentProxyResponse{}, ErrAgentProxyInvalid
	}
	event.Service, event.Method, event.RouteTemplate = request.Service, request.Request.Method, agentProxyRouteTemplate(request)
	event.Stage, event.Outcome, event.StatusCode = "credential_verification", "unavailable", 503
	if w == nil || w.credentials == nil || w.authority == nil || w.signer == nil || w.forwarder == nil {
		return AgentProxyResponse{}, ErrAgentProxyUnavailable
	}
	p, err := w.credentials.VerifyAgentCredential(ctx, credential)
	if err != nil {
		if errors.Is(err, client.ErrAgentCredentialRejected) {
			event.Outcome, event.StatusCode = "credential_rejected", 401
		}
		return AgentProxyResponse{}, err
	}
	configuration, ok := strings.CutPrefix(p.ActorID, "Agent:")
	event.Stage, event.Outcome, event.StatusCode = "identity_validation", "forbidden", 403
	if !ok || !validConfigurationID(configuration) || p.UserID == "" || p.OrganizationID == "" || p.CredentialID == "" {
		return AgentProxyResponse{}, ErrAgentProxyForbidden
	}
	p.WorkspaceID = "" // Auth's credential facts cannot assert a current Workspace.
	event.SetPrincipal(p)
	event.Stage, event.Outcome, event.StatusCode = "authority_lookup", "unavailable", 503
	facts, err := w.authority.ResolveAgentIdentity(ctx, configuration)
	if err != nil {
		if errors.Is(err, client.ErrAgentTokenForbidden) || errors.Is(err, client.ErrAgentTokenNotFound) {
			event.Outcome, event.StatusCode = "forbidden", 403
		}
		return AgentProxyResponse{}, err
	}
	if facts.ConfigurationID != configuration || facts.ActorID != p.ActorID || facts.OwnerUserID != p.UserID || facts.OrganizationID != p.OrganizationID || facts.WorkspaceID == "" || facts.TeamID == "" || !facts.OwnerAuthorized || (facts.State != "running" && facts.State != "stopped") {
		event.Outcome, event.StatusCode = "forbidden", 403
		return AgentProxyResponse{}, ErrAgentProxyForbidden
	}
	// Workspace is authoritative Control state, never a client-supplied claim.
	p.WorkspaceID = facts.WorkspaceID
	event.SetPrincipal(p)
	event.Stage, event.Outcome, event.StatusCode = "request_signing", "unavailable", 503
	assertion, err := w.signer.Sign(p, audience, request.Request)
	if err != nil {
		return AgentProxyResponse{}, ErrAgentProxyUnavailable
	}
	event.Stage, event.Outcome, event.StatusCode, event.ResultUnknown = "business_forwarding", "upstream_unavailable", 503, true
	response, resultErr = w.forwarder.ForwardAgentRequest(internaljwt.ContextWithAgentPrincipal(ctx, p), request, assertion)
	if resultErr == nil {
		event.Stage, event.Outcome, event.StatusCode, event.ResultUnknown = "business_response", delegationaudit.ResponseOutcome(response.StatusCode), response.StatusCode, false
	} else if ctx.Err() != nil {
		event.Outcome = "cancelled"
	}
	return response, resultErr
}

// Use only validated server-owned route words, never resource IDs or queries.
func agentProxyRouteTemplate(r AgentProxyRequest) string {
	u, _ := url.ParseRequestURI(r.Request.URI) // allowlist already validated it
	parts := strings.Split(strings.TrimPrefix(u.Path, "/api/v1/"), "/")
	if len(parts) > 1 && !(parts[0] == "workflows" && parts[1] == "validate") {
		parts[1] = ":id"
	}
	if len(parts) > 3 {
		parts[3] = ":ref"
	}
	return "/api/v1/" + strings.Join(parts, "/")
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
