// media_agent_gateway_e2e_test.go exercises the real HTTP/SSE chat path.
//
// Per harness/contributing/e2e.md, docker-compose owns media_auth, gateway,
// PostgreSQL, and Codex Runtime; this test only drives HTTP with a cookie jar.
//
// MEDIA_AUTH_ADDR=http://127.0.0.1:8001 \
// MEDIA_AGENT_GATEWAY_ADDR=http://127.0.0.1:8006 \
// go test ./e2e -run TestMediaAgentGatewayChatE2E -v
package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMediaAgentGatewayChatE2E(t *testing.T) {
	authAddr := os.Getenv("MEDIA_AUTH_ADDR")
	gatewayAddr := os.Getenv("MEDIA_AGENT_GATEWAY_ADDR")
	controlAddr := os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddr == "" || gatewayAddr == "" || controlAddr == "" {
		t.Skip("MEDIA_AUTH_ADDR, MEDIA_CONTROL_ADDR and MEDIA_AGENT_GATEWAY_ADDR are required; start services with docker-compose")
	}
	client := httpClient(t)
	login(t, client, authAddr, envOr("MEDIA_AUTH_E2E_USER", "smoke"), envOr("MEDIA_AUTH_E2E_PASS", "smoke123"))

	status, body := doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/workspaces", map[string]string{"name": fmt.Sprintf("e2e-%d", time.Now().UnixNano()), "agent_type": "codex"})
	if status != http.StatusCreated {
		t.Fatalf("create workspace: status=%d body=%s", status, body)
	}
	var workspace struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &workspace); err != nil {
		t.Fatal(err)
	}
	status, body = doJSON(t, client, http.MethodPost, gatewayAddr+"/api/chat/sessions", map[string]string{"workspace_id": workspace.ID, "title": "E2E session"})
	if status != http.StatusCreated {
		t.Fatalf("create session: status=%d body=%s", status, body)
	}
	var session struct {
		ID      string `json:"id"`
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatal(err)
	}
	if session.ID == "" || session.AgentID != "codex" {
		t.Fatalf("unexpected default session: %+v", session)
	}

	one := startEventSubscription(t, client, gatewayAddr, session.ID, 0)
	two := startEventSubscription(t, client, gatewayAddr, session.ID, 0)
	startTurn(t, client, gatewayAddr, session.ID, "Reply with a short greeting.")
	eventsOne := awaitEvents(t, one)
	eventsTwo := awaitEvents(t, two)
	assertCompleteEventStream(t, eventsOne)
	assertCompleteEventStream(t, eventsTwo)
	if len(eventsOne) != len(eventsTwo) {
		t.Fatalf("broadcast mismatch: first=%d second=%d", len(eventsOne), len(eventsTwo))
	}
	replayed := awaitEvents(t, startEventSubscription(t, client, gatewayAddr, session.ID, eventsOne[0].Sequence))
	if len(replayed) != len(eventsOne)-1 || replayed[0].Sequence <= eventsOne[0].Sequence {
		t.Fatalf("cursor replay mismatch: original=%+v replayed=%+v", eventsOne, replayed)
	}

	status, body = doJSON(t, client, http.MethodGet, gatewayAddr+"/api/chat/sessions/"+session.ID+"/messages", nil)
	if status != http.StatusOK {
		t.Fatalf("message history: status=%d body=%s", status, body)
	}
	var history struct {
		Messages []struct {
			Role        string `json:"role"`
			ContentJSON string `json:"content_json"`
			Sequence    int64  `json:"sequence"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatal(err)
	}
	if len(history.Messages) < 2 {
		t.Fatalf("history has %d messages, want user + assistant", len(history.Messages))
	}
	foundAssistant := false
	for _, message := range history.Messages {
		if message.Role == "assistant" && json.Valid([]byte(message.ContentJSON)) {
			foundAssistant = true
		}
	}
	if !foundAssistant {
		t.Fatalf("complete assistant message missing from history: %+v", history.Messages)
	}
}

type gatewayEvent struct {
	Type     string `json:"type"`
	Sequence int64  `json:"-"`
}

type subscription struct {
	ready  <-chan struct{}
	result <-chan []gatewayEvent
}

func startEventSubscription(t *testing.T, client *http.Client, baseURL, sessionID string, afterSequence int64) subscription {
	t.Helper()
	ready := make(chan struct{})
	result := make(chan []gatewayEvent, 1)
	go func() {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/chat/sessions/%s/events?after_sequence=%d", baseURL, sessionID, afterSequence), nil)
		if err != nil {
			t.Error(err)
			result <- nil
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Error(err)
			result <- nil
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("subscribe: status=%d body=%s", resp.StatusCode, body)
			result <- nil
			return
		}
		var events []gatewayEvent
		scanner := bufio.NewScanner(resp.Body)
		var sequence int64
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ready") {
				close(ready)
				continue
			}
			if strings.HasPrefix(line, "id:") {
				_, _ = fmt.Sscan(strings.TrimSpace(strings.TrimPrefix(line, "id:")), &sequence)
				continue
			}
			if !strings.HasPrefix(line, "data:") || sequence == 0 {
				continue
			}
			var event gatewayEvent
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
				t.Error(err)
				break
			}
			event.Sequence = sequence
			events = append(events, event)
			sequence = 0
			if event.Type == "session.ended" || event.Type == "error" {
				break
			}
		}
		if err := scanner.Err(); err != nil {
			t.Error(err)
		}
		result <- events
	}()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("event subscription did not become ready")
	}
	return subscription{ready: ready, result: result}
}

func startTurn(t *testing.T, client *http.Client, baseURL, sessionID, message string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"message": message, "idempotency_key": fmt.Sprintf("media-cli-e2e-%d", time.Now().UnixNano())})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/chat/sessions/"+sessionID+"/turns", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("run turn: status=%d body=%s", resp.StatusCode, body)
	}
	var accepted struct {
		TurnID string `json:"turn_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil || accepted.TurnID == "" {
		t.Fatalf("invalid accepted response: %+v err=%v", accepted, err)
	}
}

func awaitEvents(t *testing.T, sub subscription) []gatewayEvent {
	t.Helper()
	select {
	case events := <-sub.result:
		return events
	case <-time.After(90 * time.Second):
		t.Fatal("timed out waiting for broadcast")
		return nil
	}
}

func assertCompleteEventStream(t *testing.T, events []gatewayEvent) {
	t.Helper()
	if len(events) == 0 || events[0].Type != "session.started" {
		t.Fatalf("missing session.started: %+v", events)
	}
	if events[len(events)-1].Type != "session.ended" {
		t.Fatalf("missing session.ended: %+v", events)
	}
	for index := 1; index < len(events); index++ {
		if events[index].Sequence <= events[index-1].Sequence {
			t.Fatalf("non-increasing sequence: %+v", events)
		}
	}
}
