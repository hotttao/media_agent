// Package mcpprotocol adapts application ports to the official MCP SDK.
package mcpprotocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
)

type Caller interface {
	Call(context.Context, string, []byte) ([]byte, bool, error)
}
type Factory func(context.Context, http.Header) (Caller, int, error)
type RPCError struct {
	Code    int64
	Message string
	Data    []byte
}

func (e *RPCError) Error() string { return e.Message }
func NewHandler(factory Factory) http.Handler {
	return NewNamedHandler("media-agent-gateway", false, factory)
}
func NewNamedHandler(name string, toolsOnly bool, factory Factory) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		caller, status, e := factory(r.Context(), r.Header)
		if e != nil {
			http.Error(w, "MCP request authorization failed", status)
			return
		}
		caps := &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}}
		if !toolsOnly {
			caps.Prompts = &mcp.PromptCapabilities{}
			caps.Resources = &mcp.ResourceCapabilities{}
		}
		server := mcp.NewServer(&mcp.Implementation{Name: name, Version: "1"}, &mcp.ServerOptions{Capabilities: caps, SetCacheable: func(_ context.Context, _ mcp.Request, c *mcp.Cacheable) { c.TTLMs = 0; c.CacheScope = "private" }})
		server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
			return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				params, e := json.Marshal(req.GetParams())
				if e != nil {
					return nil, &jsonrpc.Error{Code: -32602, Message: "invalid MCP parameters"}
				}
				raw, handled, e := caller.Call(ctx, method, params)
				if e != nil {
					var rpc *RPCError
					if errors.As(e, &rpc) {
						return nil, &jsonrpc.Error{Code: rpc.Code, Message: rpc.Message, Data: rpc.Data}
					}
					return nil, &jsonrpc.Error{Code: -32000, Message: "MCP upstream unavailable"}
				}
				if !handled {
					return next(ctx, method, req)
				}
				return decodeResult(method, raw)
			}
		})
		mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}).ServeHTTP(privateResponseWriter{w}, r)
	})
}
func decodeResult(method string, raw []byte) (mcp.Result, error) {
	var result mcp.Result
	switch method {
	case "tools/list":
		result = &mcp.ListToolsResult{}
	case "tools/call":
		result = &mcp.CallToolResult{}
	case "prompts/list":
		result = &mcp.ListPromptsResult{}
	case "prompts/get":
		result = &mcp.GetPromptResult{}
	case "resources/list":
		result = &mcp.ListResourcesResult{}
	case "resources/templates/list":
		result = &mcp.ListResourceTemplatesResult{}
	case "resources/read":
		result = &mcp.ReadResourceResult{}
	default:
		return nil, &jsonrpc.Error{Code: -32601, Message: "unsupported MCP method"}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if e := decoder.Decode(result); e != nil {
		return nil, &jsonrpc.Error{Code: -32000, Message: "invalid upstream result"}
	}
	switch v := result.(type) {
	case *mcp.ListToolsResult:
		v.CacheScope = "private"
		v.TTLMs = 0
	case *mcp.ListPromptsResult:
		v.CacheScope = "private"
		v.TTLMs = 0
	case *mcp.ListResourcesResult:
		v.CacheScope = "private"
		v.TTLMs = 0
	case *mcp.ListResourceTemplatesResult:
		v.CacheScope = "private"
		v.TTLMs = 0
	case *mcp.ReadResourceResult:
		v.CacheScope = "private"
		v.TTLMs = 0
	}
	return result, nil
}

// The SDK sets transport cache headers of its own. Authorization-dependent
// responses must retain no-store at the point headers reach the network.
type privateResponseWriter struct{ http.ResponseWriter }

func (w privateResponseWriter) WriteHeader(status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(status)
}
func (w privateResponseWriter) Write(body []byte) (int, error) {
	w.Header().Set("Cache-Control", "no-store")
	return w.ResponseWriter.Write(body)
}
func (w privateResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
