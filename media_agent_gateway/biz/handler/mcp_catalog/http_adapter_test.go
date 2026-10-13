package mcp_catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"media_agent/hertz_infra/mcpprotocol"
)

type adapterCaller struct{}

func (adapterCaller) Call(_ context.Context, method string, _ []byte) ([]byte, bool, error) {
	if method == "tools/list" {
		return []byte(`{"tools":[{"name":"platform__context","inputSchema":{"type":"object"}}]}`), true, nil
	}
	return nil, false, nil
}
func TestHertzAdapterPreservesProtocolStatusHeadersAndBodies(t *testing.T) {
	handler := mcpprotocol.NewHandler(func(_ context.Context, h http.Header) (mcpprotocol.Caller, int, error) {
		if h.Get("X-Session-Token") != "verified" {
			return nil, 401, errors.New("denied")
		}
		return adapterCaller{}, 200, nil
	})
	engine := server.New()
	engine.POST("/mcp", func(ctx context.Context, c *app.RequestContext) { serveProtocol(ctx, c, handler) })
	body := `{"jsonrpc":"2.0","id":4,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"test","version":"1"}}}}`
	response := ut.PerformRequest(engine.Engine, "POST", "/mcp", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "Accept", Value: "application/json, text/event-stream"}, ut.Header{Key: "MCP-Protocol-Version", Value: "2026-07-28"}, ut.Header{Key: "MCP-Method", Value: "tools/list"}, ut.Header{Key: "X-Session-Token", Value: "verified"})
	if response.Code != 200 {
		t.Fatalf("protocol response status %d body=%s", response.Code, response.Body.String())
	}
	var result map[string]any
	if json.Unmarshal(response.Body.Bytes(), &result) != nil || result["id"] != float64(4) || result["result"] == nil {
		t.Fatal("invalid MCP response", response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cache policy missing")
	}
	denied := ut.PerformRequest(engine.Engine, "POST", "/mcp", &ut.Body{Body: strings.NewReader(body), Len: len(body)})
	if denied.Code != 401 {
		t.Fatal("authorization rejection lost", denied.Code)
	}
}
