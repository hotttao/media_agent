package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/tracecontext"
)

var ErrAgentForwardUnavailable = errors.New("Agent upstream unavailable")

type AgentProxyRequest struct {
	Service string
	Request internaljwt.AgentRequest
}
type AgentProxyResponse struct {
	StatusCode  int
	ContentType string
	Body        []byte
	TraceID     string
}
type AgentHTTPForwarder struct {
	workflowURL, materialsURL string
	http                      *http.Client
}

// Generic forwarding is deliberately separate from business route selection.
// There is no caller-controlled host, transport, header set or redirect policy.
func NewAgentHTTPForwarder(workflowURL, materialsURL string) (*AgentHTTPForwarder, error) {
	for _, address := range []string{workflowURL, materialsURL} {
		u, err := url.Parse(address)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return nil, ErrAgentForwardUnavailable
		}
	}
	return &AgentHTTPForwarder{workflowURL: strings.TrimRight(workflowURL, "/"), materialsURL: strings.TrimRight(materialsURL, "/"), http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (f *AgentHTTPForwarder) ForwardAgentRequest(ctx context.Context, r AgentProxyRequest, assertion string) (AgentProxyResponse, error) {
	if f == nil || f.http == nil || assertion == "" || strings.ContainsAny(assertion, "\r\n") || len(r.Request.Body) > 8<<20 || len(r.Request.URI) > 4096 || strings.ContainsAny(r.Request.URI+r.Request.ContentType, "\r\n\x00\\#") {
		return AgentProxyResponse{}, ErrAgentForwardUnavailable
	}
	root := ""
	switch r.Service {
	case "workflow":
		root = f.workflowURL
	case "materials":
		root = f.materialsURL
	default:
		return AgentProxyResponse{}, ErrAgentForwardUnavailable
	}
	u, err := url.ParseRequestURI(r.Request.URI)
	if err != nil || u.IsAbs() || u.Host != "" || u.RawPath != "" || !strings.HasPrefix(u.Path, "/api/v1/") {
		return AgentProxyResponse{}, ErrAgentForwardUnavailable
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return AgentProxyResponse{}, ErrAgentForwardUnavailable
		}
	}
	req, err := http.NewRequestWithContext(ctx, r.Request.Method, root+r.Request.URI, bytes.NewReader(r.Request.Body))
	if err != nil {
		return AgentProxyResponse{}, ErrAgentForwardUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+assertion)
	if r.Request.ContentType != "" {
		req.Header.Set("Content-Type", r.Request.ContentType)
	}
	tracecontext.Inject(ctx, propagation.HeaderCarrier(req.Header))
	response, err := f.http.Do(req)
	if err != nil {
		return AgentProxyResponse{}, ErrAgentForwardUnavailable
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(body) > 8<<20 {
		return AgentProxyResponse{}, ErrAgentForwardUnavailable
	}
	traceID := ""
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		traceID = span.TraceID().String()
	}
	// Never copy Set-Cookie, Location or upstream Authorization to the caller.
	return AgentProxyResponse{StatusCode: response.StatusCode, ContentType: response.Header.Get("Content-Type"), Body: body, TraceID: traceID}, nil
}
