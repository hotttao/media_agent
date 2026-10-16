package chat

import (
	"github.com/cloudwego/hertz/pkg/app"
	"media_agent/media_agent_gateway/biz/workflow"
	"strings"
	"testing"
	"time"

	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
	"media_agent/media_agent_gateway/biz/shared/client"
)

func TestSessionResponseExposesFixedContextVersion(t *testing.T) {
	now := time.Unix(1, 0).UTC()
	value := &domain.Session{
		ID: "session", WorkspaceID: "workspace", AgentID: "claude-code", AgentType: "claude-code",
		ContextVersion: 7, Status: domain.StatusActive, CreatedAt: now, UpdatedAt: now,
	}
	response := toSessionResponse(value, &client.RuntimeSession{ChatStatus: domain.ChatStatusHuman, Status: "paused", ExecutionEpoch: 3})
	if response.GetRuntimeStatus() != "paused" || response.GetExecutionEpoch() != 3 || response.GetContextVersion() != 7 || response.GetAgentType() != "claude-code" {
		t.Fatalf("response=%+v", response)
	}
}

func TestChatErrorsPreserveClientFailureWithoutLeakingAgentBody(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{workflow.ErrSessionPaused, 409},
		{&client.APIError{StatusCode: 400, Body: "private-model-credential"}, 400},
		{&client.APIError{StatusCode: 500, Body: "private-model-credential"}, 502},
	} {
		c := app.NewContext(0)
		writeError(c, tc.err)
		if c.Response.StatusCode() != tc.status || strings.Contains(string(c.Response.Body()), "private-model-credential") {
			t.Fatalf("status=%d body=%s", c.Response.StatusCode(), c.Response.Body())
		}
	}
}
