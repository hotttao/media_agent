package taskcontext

import (
	"context"
	"encoding/json"
	"log"

	"go.opentelemetry.io/otel/trace"
)

// Event deliberately has no input, output, credential or raw error fields.
type Event struct {
	Event      string `json:"event"`
	TraceID    string `json:"trace_id,omitempty"`
	JobID      string `json:"job_id,omitempty"`
	DispatchID string `json:"dispatch_id,omitempty"`
	WorkflowID string `json:"workflow_id,omitempty"`
	TaskID     string `json:"task_id,omitempty"`
	TaskType   string `json:"task_type,omitempty"`
	Status     string `json:"status,omitempty"`
}

func Log(ctx context.Context, event Event) {
	event.TraceID = ""
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		event.TraceID = span.TraceID().String()
	}
	encoded, err := json.Marshal(event)
	if err == nil {
		log.Print(string(encoded))
	}
}
