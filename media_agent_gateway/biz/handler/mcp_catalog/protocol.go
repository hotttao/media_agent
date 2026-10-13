package mcp_catalog

import (
	"context"
	"encoding/json"
	"errors"
	"media_agent/hertz_infra/mcpprotocol"
	"media_agent/media_agent_gateway/biz/workflow"
	"net/http"
	"strconv"
	"strings"
)

type protocolCaller struct {
	flow        *workflow.MCPGateway
	credentials workflow.MCPCredentials
}

func (p protocolCaller) Call(ctx context.Context, method string, params []byte) ([]byte, bool, error) {
	raw, handled, e := p.flow.Call(ctx, p.credentials, method, params)
	if e == nil {
		return raw, handled, nil
	}
	code, message := int64(-32000), "MCP service unavailable"
	var data []byte
	var reauth *workflow.MCPReauth
	switch {
	case errors.As(e, &reauth):
		code, message = -32003, "Reconnect this MCP integration in workspace settings"
		data, _ = json.Marshal(map[string]any{"error": "needs_reauth", "definition_id": reauth.Source, "retryable": false})
	case errors.Is(e, workflow.ErrMCPCursor), errors.Is(e, workflow.ErrMCPArgument):
		code, message = -32602, "Invalid parameters or expired discovery cursor"
	case errors.Is(e, workflow.ErrMCPForbidden):
		code, message = -32001, "MCP permission denied"
	case errors.Is(e, workflow.ErrMCPOutcomeUnknown):
		code, message = -32002, "MCP outcome unknown; verify before repeating a write"
		data = []byte(`{"error":"outcome_unknown","retryable":false}`)
	}
	return nil, true, &mcpprotocol.RPCError{Code: code, Message: message, Data: data}
}
func ProtocolFactory(identity *workflow.MCPIdentity, flow *workflow.MCPGateway) mcpprotocol.Factory {
	return func(ctx context.Context, h http.Header) (mcpprotocol.Caller, int, error) {
		token, ok := strings.CutPrefix(h.Get("Authorization"), "Bearer ")
		if !ok {
			return nil, 401, workflow.ErrMCPForbidden
		}
		epoch, _ := strconv.ParseInt(h.Get("X-Agent-Execution-Epoch"), 10, 64)
		c := workflow.MCPCredentials{AgentToken: token, SessionToken: h.Get("X-Session-Token"), InstanceCredential: h.Get("X-Agent-Instance-Credential"), ReplicaID: h.Get("X-Agent-Replica-ID"), IncarnationID: h.Get("X-Agent-Incarnation-ID"), TurnID: h.Get("X-Agent-Turn-ID"), ExecutionEpoch: epoch}
		if _, e := identity.Authorize(ctx, c); e != nil {
			status := 503
			if errors.Is(e, workflow.ErrMCPForbidden) {
				status = 401
			}
			return nil, status, e
		}
		return protocolCaller{flow, c}, 200, nil
	}
}
