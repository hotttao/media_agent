package mcpupstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

var ErrMCPReauth = errors.New("MCP integration needs reauthorization")
var ErrMCPUpstream = errors.New("MCP upstream request failed; outcome may be unknown")

// MCPRemoteError preserves only a protocol rejection code, never remote text or data.
type MCPRemoteError struct{ Code int64 }

func (*MCPRemoteError) Error() string { return "MCP upstream rejected request" }

type MCPBackend struct {
	Endpoint, Transport string
	Headers             map[string]string
	RequestHeaders      func(context.Context, string, json.RawMessage) (map[string]string, error)
}
type MCPUpstream struct{}
type mcpHTTPTransport struct {
	headers        map[string]string
	transport      http.RoundTripper
	reauth         *atomic.Bool
	origin         *url.URL
	requestHeaders func(context.Context, string, json.RawMessage) (map[string]string, error)
}

func (t mcpHTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.User != nil || req.URL.Scheme != t.origin.Scheme || !strings.EqualFold(req.URL.Host, t.origin.Host) {
		return nil, ErrMCPUpstream
	}
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	for key, value := range t.headers {
		clone.Header.Set(key, value)
	}
	if t.requestHeaders != nil && req.Method == http.MethodPost {
		if req.GetBody == nil {
			return nil, ErrMCPUpstream
		}
		body, err := req.GetBody()
		if err != nil {
			return nil, ErrMCPUpstream
		}
		raw, err := io.ReadAll(io.LimitReader(body, (1<<20)+1))
		body.Close()
		if err != nil || len(raw) > 1<<20 {
			return nil, ErrMCPUpstream
		}
		var message struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(raw, &message) != nil || message.Method == "" {
			return nil, ErrMCPUpstream
		}
		headers, err := t.requestHeaders(req.Context(), message.Method, message.Params)
		if err != nil {
			return nil, err
		}
		for key, value := range headers {
			clone.Header.Set(key, value)
		}
	}
	resp, e := t.transport.RoundTrip(clone)
	if e != nil {
		return nil, e
	}
	if resp.StatusCode == http.StatusUnauthorized {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if err != nil {
			return nil, ErrMCPUpstream
		}
		var value struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &value) == nil && value.Error == "needs_reauth" {
			t.reauth.Store(true)
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
	}
	resp.Body = &boundedMCPBody{ReadCloser: resp.Body, remaining: 16 << 20}
	return resp, nil
}

type boundedMCPBody struct {
	io.ReadCloser
	remaining int64
}

func (b *boundedMCPBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, ErrMCPUpstream
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, e := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	return n, e
}
func (MCPUpstream) Open(ctx context.Context, backend MCPBackend) (MCPPeer, error) {
	ctx, cancel := context.WithTimeout(mcpOutboundContext{ctx}, 30*time.Second)
	origin, e := url.Parse(backend.Endpoint)
	if e != nil || origin.Host == "" || origin.User != nil || (origin.Scheme != "http" && origin.Scheme != "https") {
		cancel()
		return nil, ErrMCPUpstream
	}
	reauth := &atomic.Bool{}
	httpClient := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: mcpHTTPTransport{headers: backend.Headers, transport: http.DefaultTransport, reauth: reauth, origin: origin, requestHeaders: backend.RequestHeaders}}
	var transport mcp.Transport
	switch backend.Transport {
	case "streamable_http":
		transport = &mcp.StreamableClientTransport{Endpoint: backend.Endpoint, HTTPClient: httpClient, MaxRetries: -1, DisableStandaloneSSE: true, MaxEventSize: 16 << 20}
	case "sse":
		transport = &mcp.SSEClientTransport{Endpoint: backend.Endpoint, HTTPClient: httpClient, MaxEventSize: 16 << 20}
	default:
		cancel()
		return nil, ErrMCPUpstream
	}
	peer, e := mcp.NewClient(&mcp.Implementation{Name: "media-agent-gateway", Version: "1"}, nil).Connect(ctx, transport, nil)
	if e != nil {
		cancel()
		if reauth.Load() {
			return nil, ErrMCPReauth
		}
		return nil, ErrMCPUpstream
	}
	return &mcpPeer{peer: peer, cancel: cancel, reauth: reauth}, nil
}

type MCPPeer interface {
	Call(context.Context, string, []byte) ([]byte, error)
	Close() error
}
type mcpPeer struct {
	peer   *mcp.ClientSession
	cancel context.CancelFunc
	reauth *atomic.Bool
}

func (p *mcpPeer) Close() error { defer p.cancel(); return p.peer.Close() }
func (p *mcpPeer) Call(ctx context.Context, method string, raw []byte) ([]byte, error) {
	ctx = mcpOutboundContext{ctx}
	peer := p.peer
	var e error
	var result any
	initialized := peer.InitializeResult()
	if initialized == nil || initialized.Capabilities == nil {
		return nil, ErrMCPUpstream
	}
	caps := initialized.Capabilities
	switch {
	case method == "tools/list" && caps.Tools == nil:
		return []byte(`{"tools":[]}`), nil
	case method == "prompts/list" && caps.Prompts == nil:
		return []byte(`{"prompts":[]}`), nil
	case method == "resources/list" && caps.Resources == nil:
		return []byte(`{"resources":[]}`), nil
	case method == "resources/templates/list" && caps.Resources == nil:
		return []byte(`{"resourceTemplates":[]}`), nil
	}
	switch method {
	case "tools/list":
		var p mcp.ListToolsParams
		if e = json.Unmarshal(raw, &p); e == nil {
			result, e = peer.ListTools(ctx, &p)
		}
	case "tools/call":
		var p mcp.CallToolParamsRaw
		if e = json.Unmarshal(raw, &p); e == nil {
			result, e = peer.CallTool(ctx, &mcp.CallToolParams{Name: p.Name, Arguments: p.Arguments})
		}
	case "prompts/list":
		var p mcp.ListPromptsParams
		if e = json.Unmarshal(raw, &p); e == nil {
			result, e = peer.ListPrompts(ctx, &p)
		}
	case "prompts/get":
		var p mcp.GetPromptParams
		if e = json.Unmarshal(raw, &p); e == nil {
			result, e = peer.GetPrompt(ctx, &p)
		}
	case "resources/list":
		var p mcp.ListResourcesParams
		if e = json.Unmarshal(raw, &p); e == nil {
			result, e = peer.ListResources(ctx, &p)
		}
	case "resources/templates/list":
		var p mcp.ListResourceTemplatesParams
		if e = json.Unmarshal(raw, &p); e == nil {
			result, e = peer.ListResourceTemplates(ctx, &p)
		}
	case "resources/read":
		var p mcp.ReadResourceParams
		if e = json.Unmarshal(raw, &p); e == nil {
			result, e = peer.ReadResource(ctx, &p)
		}
	default:
		return nil, ErrMCPUpstream
	}
	if e != nil {
		if p.reauth.Load() {
			return nil, ErrMCPReauth
		}
		var remote *jsonrpc.Error
		if errors.As(e, &remote) {
			switch remote.Code {
			case -32602, -32601, -32001:
				return nil, &MCPRemoteError{Code: remote.Code}
			}
		}
		return nil, ErrMCPUpstream
	}
	encoded, e := json.Marshal(result)
	if e != nil || len(encoded) > 16<<20 {
		return nil, ErrMCPUpstream
	}
	return encoded, nil
}

// Keep cancellation and deadlines, but never carry the inbound SDK's protocol
// version, request metadata or capabilities into an independent upstream handshake.
// Each proxy hop negotiates its own protocol and passes only explicit headers.
type mcpOutboundContext struct{ context.Context }

func (mcpOutboundContext) Value(any) any { return nil }
