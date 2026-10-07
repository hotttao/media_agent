package conductor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
)

func TestInternalRunnerRestoresInstanceTraceWithoutChangingTaskInput(t *testing.T) {
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seen := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "poll/batch"):
			json.NewEncoder(w).Encode([]Task{{TaskID: "task", WorkflowInstanceID: "workflow", TaskType: "test", InputData: map[string]any{"prompt": "private-input", "job_id": "job"}}})
		case r.URL.Path == "/api/workflow/workflow":
			json.NewEncoder(w).Encode(map[string]any{"workflowId": "workflow", "input": map[string]any{"_traceparent": parent}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/tasks":
			if !strings.HasPrefix(r.Header.Get("Traceparent"), "00-4bf92f3577b34da6a3ce929d0e0e4736-") {
				t.Error("result update lost execution trace")
			}
			w.WriteHeader(204)
			cancel()
		default:
			t.Error("unexpected native request")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	r := Runner{Client: NewClient(server.URL), WorkerID: "internal"}
	err := r.runTaskType(ctx, "test", func(ctx context.Context, input map[string]any) (map[string]any, error) {
		span := trace.SpanFromContext(ctx)
		seen <- span.SpanContext().TraceID().String() == "4bf92f3577b34da6a3ce929d0e0e4736" && !span.IsRecording() && input["_traceparent"] == nil && input["prompt"] == "private-input"
		return map[string]any{"value": "done"}, nil
	}, time.Millisecond)
	if !errors.Is(err, context.Canceled) || !<-seen {
		t.Fatal("internal execution correlation changed input or lost parent", err)
	}
}
