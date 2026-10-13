package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPUpstreamSSEAndMissingCapabilities(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "legacy", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "read", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
	})
	endpoint := httptest.NewServer(mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer endpoint.Close()
	peer, e := (MCPUpstream{}).Open(context.Background(), MCPBackend{Endpoint: endpoint.URL, Transport: "sse"})
	if e != nil {
		t.Fatal(e)
	}
	defer peer.Close()
	raw, e := peer.Call(context.Background(), "tools/list", []byte(`{}`))
	if e != nil || !bytes.Contains(raw, []byte(`"read"`)) {
		t.Fatal(string(raw), e)
	}
	for _, method := range []string{"prompts/list", "resources/list", "resources/templates/list"} {
		raw, e = peer.Call(context.Background(), method, []byte(`{}`))
		if e != nil || !bytes.Contains(raw, []byte(`[]`)) {
			t.Fatal("unsupported feature not empty", method, string(raw), e)
		}
	}
}
func TestMCPUpstreamRequiresReauthAndNeverFollowsRedirect(t *testing.T) {
	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"needs_reauth","server_origin":"not-exposed"}`))
	}))
	defer unauthorized.Close()
	if _, e := (MCPUpstream{}).Open(context.Background(), MCPBackend{Endpoint: unauthorized.URL, Transport: "streamable_http"}); !errors.Is(e, ErrMCPReauth) {
		t.Fatal(e)
	}
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	if _, e := (MCPUpstream{}).Open(context.Background(), MCPBackend{Endpoint: redirect.URL, Transport: "streamable_http"}); e == nil || calls.Load() != 0 {
		t.Fatal("redirect followed", e, calls.Load())
	}
}
func TestMCPWriteConnectionLossIsSentOnce(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "write", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "write", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	var writes atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		var request struct{ Method string }
		json.Unmarshal(body, &request)
		if request.Method == "tools/call" {
			writes.Add(1)
			conn, _, e := w.(http.Hijacker).Hijack()
			if e != nil {
				t.Error(e)
				return
			}
			conn.Close()
			return
		}
		transport.ServeHTTP(w, r)
	}))
	defer endpoint.Close()
	peer, e := (MCPUpstream{}).Open(context.Background(), MCPBackend{Endpoint: endpoint.URL, Transport: "streamable_http"})
	if e != nil {
		t.Fatal(e)
	}
	defer peer.Close()
	_, e = peer.Call(context.Background(), "tools/call", []byte(`{"name":"write"}`))
	if !errors.Is(e, ErrMCPUpstream) || writes.Load() != 1 {
		t.Fatalf("write replayed or reported success: %v calls=%d", e, writes.Load())
	}
}

func TestMCPSSERemoteMessageEndpointCannotLeakDelegation(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", target.URL)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer endpoint.Close()
	_, e := (MCPUpstream{}).Open(context.Background(), MCPBackend{Endpoint: endpoint.URL, Transport: "sse", Headers: map[string]string{"Authorization": "Bearer delegation"}})
	if e == nil || targetCalls.Load() != 0 {
		t.Fatal("SSE endpoint crossed the approved origin", e, targetCalls.Load())
	}
}
