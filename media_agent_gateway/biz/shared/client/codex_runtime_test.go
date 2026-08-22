package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestHTTPRuntimeUsesAgentRuntimeSessionAPI(t *testing.T) {
	requests := make([]string, 0, 6)
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.Method+" "+req.URL.RequestURI())
		body := `{"session_id":"session/1","status":"active","chat_status":"human","context_version":7}`
		switch req.URL.Path {
		case "/api/v1/runtime-sessions/session/1/turns":
			body = `{"turn_id":"turn-1","session_id":"session/1","status":"queued"}`
		case "/api/v1/runtime-sessions/session/1/messages":
			body = `[]`
		case "/api/v1/runtime-sessions/session/1/events":
			body = `[]`
		case "/api/v1/runtime-sessions/session/1/requests":
			body = `[]`
		case "/api/v1/runtime-sessions/session/1/turns/turn/1/requests/request/1/resolve":
			body = `{"request_id":"request/1","session_id":"session/1","turn_id":"turn/1","request_type":"permission","payload":{},"status":"resolved","response":{"option_id":"allow"}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	runtime := &HTTPRuntime{AgentType: "codex", BaseURL: "http://agent.test", HTTP: httpClient}

	if _, err := runtime.GetRuntimeSession(context.Background(), "session/1"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SubmitTurn(context.Background(), SubmitTurnInput{SessionID: "session/1", IdempotencyKey: "key", ContextVersion: 7, Input: map[string]any{"message": "hi"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListMessages(context.Background(), "session/1", 20); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ReplayEvents(context.Background(), "session/1", 4, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListAgentRequests(context.Background(), "session/1", "turn/1"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ResolveAgentRequest(context.Background(), "session/1", "turn/1", "request/1", map[string]any{"option_id": "allow"}); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"GET /api/v1/runtime-sessions/session%2F1",
		"POST /api/v1/runtime-sessions/session%2F1/turns",
		"GET /api/v1/runtime-sessions/session%2F1/messages?limit=20",
		"GET /api/v1/runtime-sessions/session%2F1/events?after_sequence=4&limit=20",
		"GET /api/v1/runtime-sessions/session%2F1/requests?turn_id=turn%2F1",
		"POST /api/v1/runtime-sessions/session%2F1/turns/turn%2F1/requests/request%2F1/resolve",
	}
	for i := range want {
		if requests[i] != want[i] {
			t.Fatalf("request[%d]=%q, want %q", i, requests[i], want[i])
		}
	}
}

func TestRuntimeRegistryRegistersBothSupportedAgentTypes(t *testing.T) {
	t.Setenv("CODEX_RUNTIME_URL", "http://codex.test")
	t.Setenv("CLAUDE_CODE_RUNTIME_URL", "http://claude.test")
	registry := NewRuntimeRegistryFromEnvironment()
	for agentType, wantURL := range map[string]string{"codex": "http://codex.test", "claude-code": "http://claude.test"} {
		runtime, ok := registry.Get(agentType)
		httpRuntime, typed := runtime.(*HTTPRuntime)
		if !ok || !typed || httpRuntime.AgentType != agentType || httpRuntime.BaseURL != wantURL {
			t.Fatalf("unexpected runtime %q: %#v", agentType, runtime)
		}
	}
}
