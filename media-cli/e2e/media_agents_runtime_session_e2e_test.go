package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMediaAgentsRuntimeSessionExecutionE2E(t *testing.T) {
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
	turnPayload := map[string]any{"idempotency_key": "e2e-request-1", "context_version": 1, "input": map[string]any{"message": "slow-success-one"}, "api_key": "e2e-secret-not-persisted"}
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
		TurnID          string `json:"turn_id"`
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
	firstDone := pollTurn(t, client, agentAddr, sessionID, first.TurnID, "completed")
	secondDone := pollTurn(t, client, agentAddr, sessionID, second.TurnID, "completed")
	if string(firstDone) == "" || string(secondDone) == "" {
		t.Fatal("completed Turns missing")
	}
	status, body = doJSON(t, client, http.MethodGet, agentAddr+"/api/v1/runtime-sessions/"+sessionID, nil)
	if status != http.StatusOK {
		t.Fatalf("get completed session: %s", body)
	}
	var completedSession struct {
		Status           string `json:"status"`
		ChatStatus       string `json:"chat_status"`
		RuntimeSessionID string `json:"runtime_session_id"`
	}
	if err := json.Unmarshal(body, &completedSession); err != nil {
		t.Fatal(err)
	}
	if completedSession.Status != "active" || completedSession.ChatStatus != "human" || completedSession.RuntimeSessionID == "" {
		t.Fatalf("unexpected completed session: %+v", completedSession)
	}
	payload["context_version"] = 2
	status, body = doJSON(t, client, http.MethodPost, agentAddr+"/api/v1/runtime-sessions", payload)
	if status != http.StatusConflict {
		t.Fatalf("context conflict: status=%d body=%s", status, body)
	}
}

func TestMediaAgentsRuntimeSessionInterruptAndFailureE2E(t *testing.T) {
	agentAddr := os.Getenv("MEDIA_AGENTS_ADDR")
	if agentAddr == "" {
		t.Skip("MEDIA_AGENTS_ADDR is required")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	sessionID := fmt.Sprintf("e2e-interrupt-%d", time.Now().UnixNano())
	register := map[string]any{"session_id": sessionID, "context_version": 1, "context_ref": "e2e:context:v1"}
	status, body := doJSON(t, client, http.MethodPost, agentAddr+"/api/v1/runtime-sessions", register)
	if status != http.StatusCreated {
		t.Fatalf("register: %s", body)
	}
	status, runningBody := doJSON(t, client, http.MethodPost, agentAddr+"/api/v1/runtime-sessions/"+sessionID+"/turns",
		map[string]any{"idempotency_key": "wait", "context_version": 1, "input": map[string]any{"message": "wait-for-interrupt"}})
	if status != http.StatusAccepted {
		t.Fatalf("submit wait: %s", runningBody)
	}
	status, queuedBody := doJSON(t, client, http.MethodPost, agentAddr+"/api/v1/runtime-sessions/"+sessionID+"/turns",
		map[string]any{"idempotency_key": "queued", "context_version": 1, "input": map[string]any{"message": "must-stay-queued"}})
	if status != http.StatusAccepted {
		t.Fatalf("submit queued: %s", queuedBody)
	}
	var running, queued struct {
		TurnID string `json:"turn_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(runningBody, &running); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(queuedBody, &queued); err != nil {
		t.Fatal(err)
	}
	status, body = doJSON(t, client, http.MethodPost,
		agentAddr+"/api/v1/runtime-sessions/"+sessionID+"/turns/"+running.TurnID+"/interrupt", map[string]any{})
	if status != http.StatusAccepted {
		t.Fatalf("interrupt: %s", body)
	}
	pollTurn(t, client, agentAddr, sessionID, running.TurnID, "interrupted")
	queuedAfter := pollTurn(t, client, agentAddr, sessionID, queued.TurnID, "queued")
	if string(queuedAfter) == "" {
		t.Fatal("queued Turn disappeared")
	}
	status, body = doJSON(t, client, http.MethodGet, agentAddr+"/api/v1/runtime-sessions/"+sessionID, nil)
	var paused struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &paused); err != nil {
		t.Fatal(err)
	}
	if paused.Status != "paused" {
		t.Fatalf("expected paused Session: %s", body)
	}

	failedSessionID := fmt.Sprintf("e2e-failure-%d", time.Now().UnixNano())
	register["session_id"] = failedSessionID
	status, body = doJSON(t, client, http.MethodPost, agentAddr+"/api/v1/runtime-sessions", register)
	if status != http.StatusCreated {
		t.Fatalf("register failure session: %s", body)
	}
	status, failedBody := doJSON(t, client, http.MethodPost,
		agentAddr+"/api/v1/runtime-sessions/"+failedSessionID+"/turns",
		map[string]any{"idempotency_key": "fail", "context_version": 1, "input": map[string]any{"message": "fail-turn"}})
	if status != http.StatusAccepted {
		t.Fatalf("submit failure: %s", failedBody)
	}
	var failedTurn struct {
		TurnID string `json:"turn_id"`
	}
	if err := json.Unmarshal(failedBody, &failedTurn); err != nil {
		t.Fatal(err)
	}
	failed := pollTurn(t, client, agentAddr, failedSessionID, failedTurn.TurnID, "failed")
	if !strings.Contains(string(failed), `"error_class":"provider_error"`) {
		t.Fatalf("missing stable error class: %s", failed)
	}
}

func pollTurn(t *testing.T, client *http.Client, agentAddr, sessionID, turnID, expected string) []byte {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		status, body := doJSON(t, client, http.MethodGet,
			agentAddr+"/api/v1/runtime-sessions/"+sessionID+"/turns/"+turnID, nil)
		if status == http.StatusOK {
			var value struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(body, &value); err != nil {
				t.Fatal(err)
			}
			if value.Status == expected {
				return body
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("Turn %s did not reach %s", turnID, expected)
	return nil
}
