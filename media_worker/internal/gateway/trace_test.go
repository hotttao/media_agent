package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkerTaskCorrelationIsNotProviderInputAndIsReusedForReports(t *testing.T) {
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "tasks:poll") {
			json.NewEncoder(w).Encode(map[string]any{"task": map[string]any{"dispatch_id": "dispatch", "attempt": 1, "input_json": `{"prompt":"rendered","images":[],"_traceparent":"` + parent + `"}`}})
			return
		}
		if r.Header.Get("Traceparent") != parent || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("Worker report correlation/identity lost")
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "submitted"})
	}))
	defer server.Close()
	c := New(server.URL, "test-token")
	task, err := c.Poll(context.Background(), "worker", Capability{Key: "cap", Version: 1})
	if err != nil || task.TraceParent != parent || task.Input["_traceparent"] != nil || strings.Contains(task.InputJSON, "traceparent") || task.TraceID() != parent[3:35] {
		t.Fatal("transport trace reached Provider inputs", err)
	}
	if err = c.Heartbeat(context.Background(), "worker", *task); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Complete(context.Background(), "worker", *task, "complete", map[string]any{"candidates": []any{}}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Fail(context.Background(), "worker", *task, "fail", "safe_failure"); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidTraceNeverBecomesAnHTTPHeader(t *testing.T) {
	for _, parent := range []string{"", "private\r\nAuthorization: secret", "00-00000000000000000000000000000000-1111111111111111-00", "00-11111111111111111111111111111111-0000000000000000-00", "00-gggggggggggggggggggggggggggggggg-1111111111111111-00"} {
		if validTraceParent(parent) || (Task{TraceParent: parent}).TraceID() != "" || TaskContext(context.Background(), Task{TraceParent: parent}).Value(taskTraceKey{}) != nil {
			t.Fatal("invalid metadata accepted")
		}
	}
}
