package taskcontext

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	"media_agent/hertz_infra/tracecontext"
)

func TestStructuredEventsDeriveTraceAndEscapeIdentity(t *testing.T) {
	var buffer bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buffer)
	defer log.SetOutput(previous)
	ctx := tracecontext.Extract(context.Background(), propagation.MapCarrier{"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"})
	Log(ctx, Event{Event: "task_complete", TraceID: "private-forged-value", JobID: "job\nforged-event"})
	line := buffer.String()
	if !strings.Contains(line, `"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"`) || strings.Contains(line, "private-forged-value") || strings.Count(line, "\n") != 1 || !strings.Contains(line, `job\nforged-event`) {
		t.Fatal("event accepted forged trace or unsafe log identity")
	}
}
