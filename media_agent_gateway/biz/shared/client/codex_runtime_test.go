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
	runtime := &CodexRuntime{BaseURL: "http://runtime.test", SSE: &sharedsse.Client{HTTP: httpClient}}
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
