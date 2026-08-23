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
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestMediaAgentGatewayCrossNodeBroadcastE2E(t *testing.T) {
	authAddr, gatewayAddr, second, controlAddr := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_AGENT_GATEWAY_ADDR"), os.Getenv("MEDIA_AGENT_GATEWAY_SECONDARY_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddr == "" || gatewayAddr == "" || second == "" || controlAddr == "" {
		t.Skip("two Gateway addresses and Auth/Control are required")
	}
	client := registerGatewayUser(t, authAddr, "crossnode")
	sessionID := createGatewaySession(t, client, controlAddr, gatewayAddr, "cross-node")
	one := startEventSubscription(t, client, gatewayAddr, sessionID, 0)
	two := startEventSubscription(t, client, second, sessionID, 0)
	startTurn(t, client, gatewayAddr, sessionID, "Reply with one short sentence.")
	eventsOne, eventsTwo := awaitEvents(t, one), awaitEvents(t, two)
	assertCompleteEventStream(t, eventsOne)
	assertCompleteEventStream(t, eventsTwo)
	if len(eventsOne) != len(eventsTwo) {
		t.Fatalf("cross-node streams differ: %d != %d", len(eventsOne), len(eventsTwo))
	}
}

func TestMediaAgentGatewayRecoversWhenRedisPublishFailsE2E(t *testing.T) {
	authAddr, gatewayAddr, controlAddr, redisContainer := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_AGENT_GATEWAY_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR"), os.Getenv("MEDIA_AGENT_GATEWAY_E2E_REDIS_CONTAINER")
	if authAddr == "" || gatewayAddr == "" || controlAddr == "" || redisContainer == "" {
		t.Skip("Gateway stack and Redis container name are required")
	}
	client := registerGatewayUser(t, authAddr, "redisdown")
	sessionID := createGatewaySession(t, client, controlAddr, gatewayAddr, "redis-down")
	feed := startEventSubscription(t, client, gatewayAddr, sessionID, 0)
	if output, err := exec.Command("docker", "stop", redisContainer).CombinedOutput(); err != nil {
		t.Fatalf("stop Redis: %v: %s", err, output)
	}
	defer exec.Command("docker", "start", redisContainer).Run()
	startTurn(t, client, gatewayAddr, sessionID, "Reply briefly.")
	events := awaitEvents(t, feed)
	assertCompleteEventStream(t, events)
}

func registerGatewayUser(t *testing.T, authAddr, prefix string) *http.Client {
	t.Helper()
	client := httpClient(t)
	username := fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	status, body := doJSON(t, client, http.MethodPost, authAddr+"/api/auth/register", map[string]string{"username": username, "password": "gateway123", "display_name": "Gateway E2E"})
	if status != http.StatusCreated {
		t.Fatalf("register user: status=%d body=%s", status, body)
	}
	return client
}

func createGatewaySession(t *testing.T, client *http.Client, controlAddr, gatewayAddr, name string) string {
	t.Helper()
	status, body := doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/workspaces", map[string]string{"name": fmt.Sprintf("%s-%d", name, time.Now().UnixNano()), "agent_type": "codex"})
	if status != http.StatusCreated {
		t.Fatalf("create workspace: status=%d body=%s", status, body)
	}
	var workspace struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &workspace); err != nil {
		t.Fatal(err)
	}
	status, body = doJSON(t, client, http.MethodPost, gatewayAddr+"/api/chat/sessions", map[string]string{"workspace_id": workspace.ID, "title": name})
	if status != http.StatusCreated {
		t.Fatalf("create session: status=%d body=%s", status, body)
	}
	var session struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatal(err)
	}
	return session.ID
}

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

func TestMediaAgentGatewayClaudeCodeChatE2E(t *testing.T) {
	authAddr := os.Getenv("MEDIA_AUTH_ADDR")
	gatewayAddr := os.Getenv("MEDIA_AGENT_GATEWAY_ADDR")
	controlAddr := os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddr == "" || gatewayAddr == "" || controlAddr == "" {
		t.Skip("MEDIA_AUTH_ADDR, MEDIA_CONTROL_ADDR and MEDIA_AGENT_GATEWAY_ADDR are required")
	}
	client := httpClient(t)
	username := fmt.Sprintf("claudechat%d", time.Now().UnixNano())
	status, body := doJSON(t, client, http.MethodPost, authAddr+"/api/auth/register", map[string]string{
		"username": username, "password": "claudechat123", "display_name": "Claude Chat E2E",
	})
	if status != http.StatusCreated {
		t.Fatalf("register E2E user: status=%d body=%s", status, body)
	}

	status, body = doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/workspaces", map[string]string{
		"name": "Claude Chat", "agent_type": "claude-code", "api_key": "e2e-secret",
	})
	if status != http.StatusCreated {
		t.Fatalf("create Claude workspace: status=%d body=%s", status, body)
	}
	var workspace struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &workspace); err != nil {
		t.Fatal(err)
	}
	status, body = doJSON(t, client, http.MethodPost, gatewayAddr+"/api/chat/sessions", map[string]string{
		"workspace_id": workspace.ID, "title": "Claude session",
	})
	if status != http.StatusCreated {
		t.Fatalf("create Claude session: status=%d body=%s", status, body)
	}
	var session struct {
		ID        string `json:"id"`
		AgentType string `json:"agent_type"`
	}
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatal(err)
	}
	if session.AgentType != "claude-code" {
		t.Fatalf("session Agent Type = %q", session.AgentType)
	}

	subscription := startEventSubscription(t, client, gatewayAddr, session.ID, 0)
	startTurn(t, client, gatewayAddr, session.ID, "hello-through-gateway")
	events := awaitEvents(t, subscription)
	assertCompleteEventStream(t, events)

	status, body = doJSON(t, client, http.MethodGet, gatewayAddr+"/api/chat/sessions/"+session.ID+"/messages", nil)
	if status != http.StatusOK {
		t.Fatalf("Claude history: status=%d body=%s", status, body)
	}
	if !bytes.Contains(body, []byte("Claude fixture: hello-through-gateway")) {
		t.Fatalf("Claude assistant message missing from history: %s", body)
	}
	if bytes.Contains(body, []byte("e2e-secret")) {
		t.Fatalf("Claude history exposed API Key: %s", body)
	}
}

func TestMediaAgentGatewayAgentConfigurationSwitchE2E(t *testing.T) {
	authAddr := os.Getenv("MEDIA_AUTH_ADDR")
	gatewayAddr := os.Getenv("MEDIA_AGENT_GATEWAY_ADDR")
	controlAddr := os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddr == "" || gatewayAddr == "" || controlAddr == "" {
		t.Skip("MEDIA_AUTH_ADDR, MEDIA_CONTROL_ADDR and MEDIA_AGENT_GATEWAY_ADDR are required")
	}
	client := registerGatewayUser(t, authAddr, "agentswitch")
	status, body := doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/workspaces", map[string]string{
		"name": "Agent switch", "agent_type": "codex", "api_key": "codex-key",
	})
	if status != http.StatusCreated {
		t.Fatalf("create Workspace: status=%d body=%s", status, body)
	}
	var workspace struct {
		ID                   string `json:"id"`
		ConfigurationVersion int64  `json:"configuration_version"`
	}
	if err := json.Unmarshal(body, &workspace); err != nil {
		t.Fatal(err)
	}
	status, body = doJSON(t, client, http.MethodPost, gatewayAddr+"/api/chat/sessions", map[string]string{
		"workspace_id": workspace.ID, "title": "Codex session",
	})
	if status != http.StatusCreated {
		t.Fatalf("create old Session: status=%d body=%s", status, body)
	}
	var oldSession struct {
		ID             string `json:"id"`
		AgentType      string `json:"agent_type"`
		ContextVersion int64  `json:"context_version"`
	}
	if err := json.Unmarshal(body, &oldSession); err != nil {
		t.Fatal(err)
	}
	if oldSession.ContextVersion != workspace.ConfigurationVersion || oldSession.AgentType != "codex" {
		t.Fatalf("old Session=%+v Workspace=%+v", oldSession, workspace)
	}

	status, body = doJSON(t, client, http.MethodPut, controlAddr+"/api/v1/workspaces/"+workspace.ID, map[string]string{
		"name": "Agent switch", "agent_type": "claude-code", "api_key": "claude-key",
	})
	if status != http.StatusOK {
		t.Fatalf("switch Agent: status=%d body=%s", status, body)
	}
	var switched struct {
		ConfigurationVersion int64 `json:"configuration_version"`
	}
	if err := json.Unmarshal(body, &switched); err != nil {
		t.Fatal(err)
	}
	if switched.ConfigurationVersion <= oldSession.ContextVersion {
		t.Fatalf("configuration version did not advance: old=%d new=%d", oldSession.ContextVersion, switched.ConfigurationVersion)
	}

	status, body = doJSON(t, client, http.MethodPost, gatewayAddr+"/api/chat/sessions/"+oldSession.ID+"/turns", map[string]string{
		"message": "must not run", "idempotency_key": "stale-session",
	})
	if status != http.StatusConflict || !bytes.Contains(body, []byte("create a new session")) {
		t.Fatalf("stale Session Turn: status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodGet, gatewayAddr+"/api/chat/sessions/"+oldSession.ID+"/messages", nil)
	if status != http.StatusOK {
		t.Fatalf("old Session history: status=%d body=%s", status, body)
	}

	status, body = doJSON(t, client, http.MethodPost, gatewayAddr+"/api/chat/sessions", map[string]string{
		"workspace_id": workspace.ID, "title": "Claude session",
	})
	if status != http.StatusCreated {
		t.Fatalf("create compatible Session: status=%d body=%s", status, body)
	}
	var newSession struct {
		ID             string `json:"id"`
		AgentType      string `json:"agent_type"`
		ContextVersion int64  `json:"context_version"`
	}
	if err := json.Unmarshal(body, &newSession); err != nil {
		t.Fatal(err)
	}
	if newSession.ID == oldSession.ID || newSession.AgentType != "claude-code" || newSession.ContextVersion != switched.ConfigurationVersion {
		t.Fatalf("new Session=%+v switched Workspace=%+v", newSession, switched)
	}
}

func TestMediaAgentGatewayInterruptTurnE2E(t *testing.T) {
	authAddr := os.Getenv("MEDIA_AUTH_ADDR")
	gatewayAddr := os.Getenv("MEDIA_AGENT_GATEWAY_ADDR")
	controlAddr := os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddr == "" || gatewayAddr == "" || controlAddr == "" {
		t.Skip("MEDIA_AUTH_ADDR, MEDIA_CONTROL_ADDR and MEDIA_AGENT_GATEWAY_ADDR are required; start services with docker-compose")
	}
	client := httpClient(t)
	login(t, client, authAddr, envOr("MEDIA_AUTH_E2E_USER", "smoke"), envOr("MEDIA_AUTH_E2E_PASS", "smoke123"))

	status, body := doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/workspaces", map[string]string{"name": fmt.Sprintf("interrupt-e2e-%d", time.Now().UnixNano()), "agent_type": "codex"})
	if status != http.StatusCreated {
		t.Fatalf("create workspace: status=%d body=%s", status, body)
	}
	var workspace struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &workspace); err != nil {
		t.Fatal(err)
	}
	status, body = doJSON(t, client, http.MethodPost, gatewayAddr+"/api/chat/sessions", map[string]string{"workspace_id": workspace.ID, "title": "Interrupt E2E"})
	if status != http.StatusCreated {
		t.Fatalf("create session: status=%d body=%s", status, body)
	}
	var session struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatal(err)
	}

	feed := startEventFeed(t, client, gatewayAddr, session.ID)
	startTurn(t, client, gatewayAddr, session.ID, "Use the shell to run `sleep 120`, wait for it to finish, then reply done.")
	observedTransient := false
	for {
		event := awaitFeedEvent(t, feed)
		if event.Sequence == 0 && (event.Type == "item.started" || event.Type == "item.delta") {
			observedTransient = true
			break
		}
		if event.Type == "error" || event.Type == "session.ended" {
			t.Fatalf("turn ended before interrupt: %+v", event)
		}
	}
	status, body = doJSON(t, client, http.MethodPost, gatewayAddr+"/api/chat/sessions/"+session.ID+"/interrupt", map[string]string{})
	if status != http.StatusAccepted {
		t.Fatalf("interrupt: status=%d body=%s", status, body)
	}
	var interrupted bool
	for !interrupted {
		event := awaitFeedEvent(t, feed)
		if event.Type == "session.ended" {
			if event.Reason != "interrupted" {
				t.Fatalf("terminal reason=%q, want interrupted", event.Reason)
			}
			interrupted = true
		}
	}
	if !observedTransient {
		t.Fatal("no transient Runtime frame was observed before interrupt")
	}

	status, body = doJSON(t, client, http.MethodGet, gatewayAddr+"/api/chat/sessions?workspace_id="+workspace.ID, nil)
	if status != http.StatusOK {
		t.Fatalf("list sessions: status=%d body=%s", status, body)
	}
	var sessions struct {
		Sessions []struct {
			ID         string `json:"id"`
			ChatStatus string `json:"chat_status"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(body, &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions.Sessions) != 1 || sessions.Sessions[0].ChatStatus != "human" {
		t.Fatalf("chat status was not restored: %+v", sessions.Sessions)
	}

	replayed := awaitEvents(t, startEventSubscription(t, client, gatewayAddr, session.ID, 0))
	for _, event := range replayed {
		if event.Sequence == 0 || event.Type == "item.started" || event.Type == "item.delta" {
			t.Fatalf("transient event was persisted/replayed: %+v", event)
		}
	}
}

type gatewayEvent struct {
	Type     string `json:"type"`
	Reason   string `json:"reason"`
	Sequence int64  `json:"-"`
}

type eventFeed struct {
	events <-chan gatewayEvent
}

func startEventFeed(t *testing.T, client *http.Client, baseURL, sessionID string) eventFeed {
	t.Helper()
	ready := make(chan struct{})
	events := make(chan gatewayEvent, 32)
	go func() {
		defer close(events)
		req, err := http.NewRequest(http.MethodGet, baseURL+"/api/chat/sessions/"+sessionID+"/events?after_sequence=0", nil)
		if err != nil {
			t.Error(err)
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Error(err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("subscribe feed: status=%d body=%s", resp.StatusCode, body)
			return
		}
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
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var event gatewayEvent
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
				continue
			}
			event.Sequence = sequence
			sequence = 0
			events <- event
			if event.Type == "session.ended" || event.Type == "error" {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("event feed did not become ready")
	}
	return eventFeed{events: events}
}

func awaitFeedEvent(t *testing.T, feed eventFeed) gatewayEvent {
	t.Helper()
	select {
	case event, ok := <-feed.events:
		if !ok {
			t.Fatal("event feed closed before terminal event")
		}
		return event
	case <-time.After(90 * time.Second):
		t.Fatal("timed out waiting for feed event")
		return gatewayEvent{}
	}
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
	if len(events) == 0 {
		t.Fatal("event stream is empty")
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
