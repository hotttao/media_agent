package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMediaAgentsRuntimeSessionRegistrationE2E(t *testing.T) {
	agentAddr := os.Getenv("MEDIA_AGENTS_ADDR")
	if agentAddr == "" {
		t.Skip("MEDIA_AGENTS_ADDR is required; start PostgreSQL and an Agent with docker-compose")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	sessionID := fmt.Sprintf("e2e-runtime-%d", time.Now().UnixNano())
	payload := map[string]any{
		"session_id": sessionID, "context_version": 1, "context_ref": "e2e:context:v1",
	}

	status, body := doJSON(t, client, http.MethodPost, agentAddr+"/api/v1/runtime-sessions", payload)
	if status != http.StatusCreated {
		t.Fatalf("register runtime session: status=%d body=%s", status, body)
	}
	status, repeated := doJSON(t, client, http.MethodPost, agentAddr+"/api/v1/runtime-sessions", payload)
	if status != http.StatusCreated || string(repeated) != string(body) {
		t.Fatalf("idempotent register: status=%d first=%s repeated=%s", status, body, repeated)
	}
	status, body = doJSON(t, client, http.MethodGet, agentAddr+"/api/v1/runtime-sessions/"+sessionID, nil)
	if status != http.StatusOK {
		t.Fatalf("get runtime session: status=%d body=%s", status, body)
	}
	var session struct {
		SessionID      string `json:"session_id"`
		ContextVersion int64  `json:"context_version"`
		Status         string `json:"status"`
		ChatStatus     string `json:"chat_status"`
	}
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatal(err)
	}
	if session.SessionID != sessionID || session.ContextVersion != 1 || session.Status != "active" || session.ChatStatus != "human" {
		t.Fatalf("unexpected runtime session: %+v", session)
	}
	turnPayload := map[string]any{"idempotency_key": "e2e-request-1", "context_version": 1, "input": map[string]any{"text": "hello"}}
	status, firstTurn := doJSON(t, client, http.MethodPost, agentAddr+"/api/v1/runtime-sessions/"+sessionID+"/turns", turnPayload)
	if status != http.StatusAccepted {
		t.Fatalf("submit turn: status=%d body=%s", status, firstTurn)
	}
	status, repeatedTurn := doJSON(t, client, http.MethodPost, agentAddr+"/api/v1/runtime-sessions/"+sessionID+"/turns", turnPayload)
	if status != http.StatusAccepted || string(firstTurn) != string(repeatedTurn) {
		t.Fatalf("idempotent turn mismatch")
	}
	turnPayload["idempotency_key"] = "e2e-request-2"
	status, queuedTurn := doJSON(t, client, http.MethodPost, agentAddr+"/api/v1/runtime-sessions/"+sessionID+"/turns", turnPayload)
	if status != http.StatusAccepted {
		t.Fatalf("queue turn: status=%d body=%s", status, queuedTurn)
	}
	var first, second struct {
		Status          string `json:"status"`
		EnqueueSequence int64  `json:"enqueue_sequence"`
	}
	if err := json.Unmarshal(firstTurn, &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(queuedTurn, &second); err != nil {
		t.Fatal(err)
	}
	if first.Status != "running" || second.Status != "queued" || first.EnqueueSequence != 1 || second.EnqueueSequence != 2 {
		t.Fatalf("unexpected FIFO: first=%+v second=%+v", first, second)
	}
	payload["context_version"] = 2
	status, body = doJSON(t, client, http.MethodPost, agentAddr+"/api/v1/runtime-sessions", payload)
	if status != http.StatusConflict {
		t.Fatalf("context conflict: status=%d body=%s", status, body)
	}
}
