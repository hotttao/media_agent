package chat

import (
	"testing"
	"time"

	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
)

func TestSessionResponseExposesFixedContextVersion(t *testing.T) {
	now := time.Unix(1, 0).UTC()
	value := &domain.Session{
		ID: "session", WorkspaceID: "workspace", AgentID: "claude-code", AgentType: "claude-code",
		ContextVersion: 7, Status: domain.StatusActive, CreatedAt: now, UpdatedAt: now,
	}
	response := toSessionResponse(value, domain.ChatStatusHuman)
	if response.GetContextVersion() != 7 || response.GetAgentType() != "claude-code" {
		t.Fatalf("response=%+v", response)
	}
}
