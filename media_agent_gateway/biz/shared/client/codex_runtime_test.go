package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	sharedsse "media_agent/media_agent_gateway/biz/shared/sse"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestCodexRuntimeConsumesSSEInOrder(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Path != "/chat" {
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
		}
		stream := "event: message\ndata: {\"type\":\"session.started\"}\n\nevent: message\ndata: {\"type\":\"session.ended\",\"reason\":\"completed\"}\n\n"
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
	})}

	var eventTypes []string
	runtime := &HTTPRuntime{AgentType: "codex", BaseURL: "http://runtime.test", SSE: &sharedsse.Client{HTTP: httpClient}}
	err := runtime.Chat(context.Background(), ChatInput{Message: "hello"}, func(raw json.RawMessage) error {
		var event struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			return err
		}
		eventTypes = append(eventTypes, event.Type)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(eventTypes) != 2 || eventTypes[0] != "session.started" || eventTypes[1] != "session.ended" {
		t.Fatalf("unexpected events: %v", eventTypes)
	}
}

func TestCodexRuntimeInterruptsRuntimeSession(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Path != "/sessions/runtime-1/interrupt" {
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"success":true,"interrupted":true}`))}, nil
	})}
	runtime := &HTTPRuntime{AgentType: "codex", BaseURL: "http://runtime.test", HTTP: httpClient}
	ok, err := runtime.Interrupt(context.Background(), "runtime-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("interrupt was not accepted")
	}
}

func TestRuntimeRegistryRegistersBothSupportedAgentTypes(t *testing.T) {
	t.Setenv("CODEX_RUNTIME_URL", "http://codex.test")
	t.Setenv("CLAUDE_CODE_RUNTIME_URL", "http://claude.test")
	registry := NewRuntimeRegistryFromEnvironment()

	for agentType, wantURL := range map[string]string{
		"codex": "http://codex.test", "claude-code": "http://claude.test",
	} {
		runtime, ok := registry.Get(agentType)
		if !ok {
			t.Fatalf("runtime %q is not registered", agentType)
		}
		httpRuntime, ok := runtime.(*HTTPRuntime)
		if !ok || httpRuntime.AgentType != agentType || httpRuntime.BaseURL != wantURL {
			t.Fatalf("unexpected runtime %q: %#v", agentType, runtime)
		}
	}
}
