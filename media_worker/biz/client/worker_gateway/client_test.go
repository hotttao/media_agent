package worker_gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"media_agent/media_worker/biz/shared/worker_runtime"
)

func TestClientPollUsesBearerTokenAndDecodesOpaqueTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/worker/v1/tasks:poll" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer talos_test_secret" {
			t.Fatalf("Authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["worker_instance_id"] != "desktop-1" || body["capability_key"] != "media.image.model" {
			t.Fatalf("body = %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"task": map[string]any{
			"dispatch_id": "dispatch-1", "attempt": 2, "capability_key": "media.image.model",
			"capability_version": 1, "input_json": `{"step_key":"model_image","prompt":"hello"}`,
		}})
	}))
	defer server.Close()

	task, err := New(server.URL, "talos_test_secret").Poll(context.Background(), "desktop-1", worker_runtime.Capability{Key: "media.image.model", Version: 1})
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if task.DispatchID != "dispatch-1" || task.Attempt != 2 || task.Input["step_key"] != "model_image" {
		t.Fatalf("task = %#v", task)
	}
}

func TestClientReturnsTypedHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "expired", http.StatusGone)
	}))
	defer server.Close()

	_, err := New(server.URL, "talos_test_secret").Complete(context.Background(), "desktop-1", worker_runtime.Task{DispatchID: "dispatch-1", Attempt: 1}, "key-1", map[string]any{"candidates": []any{}})
	if !errors.Is(err, worker_runtime.ErrResultRejected) {
		t.Fatalf("Complete() error = %#v", err)
	}
}

func TestClientHTTPErrorDoesNotRetainResponseSecrets(t *testing.T) {
	const secret = "test-only-response-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, secret, http.StatusUnauthorized)
	}))
	defer server.Close()
	err := New(server.URL, "test-only-token").Register(context.Background(), "instance", nil)
	var typed *HTTPError
	if !errors.As(err, &typed) || typed.StatusCode != http.StatusUnauthorized || strings.Contains(err.Error(), secret) {
		t.Fatal("HTTP status lost or upstream body leaked")
	}
}

func TestClientPollNoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	task, err := New(server.URL, "talos_test_secret").Poll(context.Background(), "desktop-1", worker_runtime.Capability{Key: "media.image.model", Version: 1})
	if err != nil || task != nil {
		t.Fatalf("Poll() = %#v, %v", task, err)
	}
}

func TestClientCompleteUsesDispatchRouteAndStablePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/worker/v1/tasks/dispatch-1/complete" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var body struct {
			WorkerID       string `json:"worker_instance_id"`
			Attempt        int    `json:"attempt"`
			IdempotencyKey string `json:"idempotency_key"`
			OutputJSON     string `json:"output_json"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.WorkerID != "desktop-1" || body.Attempt != 3 || body.IdempotencyKey != "key-1" {
			t.Fatalf("body = %#v", body)
		}
		var output map[string]any
		if err := json.Unmarshal([]byte(body.OutputJSON), &output); err != nil || output["candidates"] == nil {
			t.Fatalf("output_json = %q, err = %v", body.OutputJSON, err)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"receipt_id": "receipt-1", "dispatch_id": "dispatch-1", "status": "submitted"})
	}))
	defer server.Close()

	receipt, err := New(server.URL, "talos_test_secret").Complete(context.Background(), "desktop-1", worker_runtime.Task{DispatchID: "dispatch-1", Attempt: 3}, "key-1", map[string]any{"candidates": []any{}})
	if err != nil || receipt.ReceiptID != "receipt-1" {
		t.Fatalf("Complete() = %#v, %v", receipt, err)
	}
}

func TestGatewayWireRegistrationAndResultSemantics(t *testing.T) {
	for _, status := range []int{http.StatusConflict, http.StatusGone} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		_, err := New(server.URL, "token").Complete(context.Background(), "worker", worker_runtime.Task{DispatchID: "dispatch", Attempt: 1}, "key", map[string]any{"candidates": []any{}})
		server.Close()
		if !errors.Is(err, worker_runtime.ErrResultRejected) {
			t.Fatalf("status %d did not map to domain rejection: %v", status, err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "registrations") {
			var body struct {
				Capabilities []map[string]any `json:"capabilities"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Capabilities) != 1 {
				t.Error("registration malformed")
				return
			}
			cap := body.Capabilities[0]
			if cap["capability_key"] != "cap" || cap["capability_version"] != float64(2) || cap["concurrency"] != float64(3) || len(cap) != 3 {
				t.Error("wire capability names changed")
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(`{"receipt_id":"r","status":"pending"}`))
	}))
	defer server.Close()
	c := New(server.URL, "token")
	if err := c.Register(context.Background(), "worker", []worker_runtime.Capability{{Key: "cap", Version: 2, Concurrency: 3}}); err != nil {
		t.Fatal(err)
	}
	task := worker_runtime.Task{DispatchID: "dispatch", Attempt: 1}
	if _, err := c.Complete(context.Background(), "worker", task, "key", map[string]any{}); err == nil {
		t.Fatal("unconfirmed completion accepted")
	}
	if _, err := c.FailWithKind(context.Background(), "worker", task, "key", "failed", "permanent"); err == nil {
		t.Fatal("unconfirmed failure accepted")
	}
}
