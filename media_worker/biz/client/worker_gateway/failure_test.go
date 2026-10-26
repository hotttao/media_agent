package worker_gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"media_agent/media_worker/biz/shared/worker_runtime"
)

func TestFailWithKindSendsStableClassifiedReceipt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Kind   string `json:"failure_kind"`
			Key    string `json:"idempotency_key"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Kind != "transient" || body.Key != "stable-key" || body.Reason != "provider unavailable" {
			t.Error("failure contract was not preserved")
		}
		if r.URL.Path != "/api/worker/v1/tasks/d1/fail" {
			t.Error("unexpected route")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"receipt_id":"r1","status":"submitted"}`))
	}))
	defer server.Close()
	client := New(server.URL, "test-token")
	for range 2 {
		receipt, err := client.FailWithKind(context.Background(), "worker-1", worker_runtime.Task{DispatchID: "d1", Attempt: 1}, "stable-key", "provider unavailable", "transient")
		if err != nil || receipt.ReceiptID != "r1" {
			t.Fatalf("receipt=%#v err=%v", receipt, err)
		}
	}
}
