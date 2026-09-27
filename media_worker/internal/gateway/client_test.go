package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientPollUsesBearerTokenAndDecodesOpaqueTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/worker/v1/tasks:poll" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer service-secret" {
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

	task, err := New(server.URL, "service-secret").Poll(context.Background(), "desktop-1", Capability{Key: "media.image.model", Version: 1})
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

	_, err := New(server.URL, "service-secret").Complete(context.Background(), "desktop-1", Task{DispatchID: "dispatch-1", Attempt: 1}, "key-1", map[string]any{"candidates": []any{}})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusGone {
		t.Fatalf("Complete() error = %#v", err)
	}
}

func TestClientPollNoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	task, err := New(server.URL, "service-secret").Poll(context.Background(), "desktop-1", Capability{Key: "media.image.model", Version: 1})
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
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"receipt_id": "receipt-1", "dispatch_id": "dispatch-1", "status": "pending"})
	}))
	defer server.Close()

	receipt, err := New(server.URL, "service-secret").Complete(context.Background(), "desktop-1", Task{DispatchID: "dispatch-1", Attempt: 3}, "key-1", map[string]any{"candidates": []any{}})
	if err != nil || receipt.ReceiptID != "receipt-1" {
		t.Fatalf("Complete() = %#v, %v", receipt, err)
	}
}
