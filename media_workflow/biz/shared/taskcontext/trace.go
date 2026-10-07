package taskcontext

import (
	"context"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"media_agent/hertz_infra/tracecontext"
)

const TraceParentKey = "_traceparent"

// PreserveTrace carries a valid original execution parent into a whole rerun;
// the triggering request's trace does not replace the execution's identity.
func PreserveTrace(original, replacement map[string]any) map[string]any {
	parent, _ := original[TraceParentKey].(string)
	ctx := tracecontext.Extract(context.Background(), propagation.MapCarrier{"traceparent": parent})
	return CaptureTrace(ctx, replacement)
}

// CaptureTrace copies the transport envelope. It never changes business input,
// forwards baggage or overwrites metadata when replaying a frozen Start.
func CaptureTrace(ctx context.Context, input map[string]any) map[string]any {
	copy := make(map[string]any, len(input)+1)
	for key, value := range input {
		copy[key] = value
	}
	carrier := propagation.MapCarrier{}
	tracecontext.Inject(ctx, carrier)
	if parent := carrier.Get("traceparent"); parent != "" {
		copy[TraceParentKey] = parent
	}
	return copy
}

// StartExecution works with both traced and legacy envelopes. Invalid input
// cannot grant identity or invalidate a native Task; only a trace is restored.
func StartExecution(ctx context.Context, input map[string]any, name string) (context.Context, trace.Span) {
	parent, _ := input[TraceParentKey].(string)
	ctx = tracecontext.Extract(ctx, propagation.MapCarrier{"traceparent": parent})
	return tracecontext.Start(ctx, name, trace.SpanKindConsumer)
}
