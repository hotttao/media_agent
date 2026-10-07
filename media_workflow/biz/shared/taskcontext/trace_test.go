package taskcontext

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"media_agent/hertz_infra/tracecontext"
)

func TestFrozenTraceRestoresAsyncParentWithoutBusinessMutation(t *testing.T) {
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	ctx := tracecontext.Extract(context.Background(), propagation.MapCarrier{"traceparent": parent, "baggage": "private=value", "tracestate": "private=value"})
	input := map[string]any{"job_id": "job", "inputs": map[string]any{"text": "business"}}
	frozen := CaptureTrace(ctx, input)
	if frozen[TraceParentKey] != parent || input[TraceParentKey] != nil || len(frozen) != 3 {
		t.Fatal("capture mutated input or propagated extra metadata")
	}
	consumer, span := StartExecution(context.Background(), frozen, "async-start")
	defer span.End()
	got := trace.SpanContextFromContext(consumer)
	if got.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" || got.SpanID().String() == "00f067aa0ba902b7" || got.IsSampled() || span.IsRecording() {
		t.Fatal("async parent lost or unexpected recording/export")
	}
}

func TestMissingOrInvalidTraceDoesNotBlockExecution(t *testing.T) {
	for _, parent := range []any{nil, "malformed", "00-00000000000000000000000000000000-00f067aa0ba902b7-01", 42} {
		ctx, span := StartExecution(context.Background(), map[string]any{TraceParentKey: parent}, "legacy")
		if !trace.SpanContextFromContext(ctx).IsValid() || span.IsRecording() {
			t.Fatal("legacy tracing failed")
		}
		span.End()
	}
}

func TestWholeRerunPreservesOnlyValidExecutionTrace(t *testing.T) {
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	for _, value := range []any{parent, "invalid", nil, 42} {
		original := map[string]any{TraceParentKey: value, "inputs": "old"}
		replacement := map[string]any{"inputs": "new"}
		result := PreserveTrace(original, replacement)
		if result["inputs"] != "new" || replacement[TraceParentKey] != nil || original["inputs"] != "old" {
			t.Fatal("rerun mutated business envelopes")
		}
		if value == parent && result[TraceParentKey] != parent || value != parent && result[TraceParentKey] != nil {
			t.Fatal("rerun lost valid trace or propagated invalid metadata")
		}
	}
}
