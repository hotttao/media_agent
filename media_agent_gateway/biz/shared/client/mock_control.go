package client

import (
	"context"
	service "media_agent/media_agent_gateway/biz/service/agent_session"
)

// MockControl is the minimum stable media_control seam for local chat. It can
// be replaced by an RPC-backed provider without changing domain or workflow.
type MockControl struct{ WorkspaceID, AgentID string }

func (m MockControl) Default(context.Context, string) (service.AgentContext, error) {
	workspace := m.WorkspaceID
	if workspace == "" {
		workspace = "default"
	}
	agent := m.AgentID
	if agent == "" {
		agent = "codex"
	}
	return service.AgentContext{WorkspaceID: workspace, AgentID: agent, AgentType: agent, ContextVersion: 1}, nil
}

func (m MockControl) Resolve(ctx context.Context, _ string, workspaceID, _ string) (service.AgentContext, error) {
	m.WorkspaceID = workspaceID
	return m.Default(ctx, "")
}
