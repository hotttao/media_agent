// Package tracecontext provides request correlation without an exporter or
// global tracer provider. Local spans are never recorded or sampled.
package tracecontext

import (
	"context"

	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var localTracer = sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample())).Tracer("media-agent/local-correlation")

// Start creates a local span while preserving a valid parent trace ID. No
// exporter, processor or global provider is installed, even for sampled parents.
func Start(ctx context.Context, name string, kind trace.SpanKind) (context.Context, trace.Span) {
	return localTracer.Start(ctx, name, trace.WithSpanKind(kind))
}

// Extract accepts only the standard W3C trace context, not credentials or baggage.
func Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return propagation.TraceContext{}.Extract(ctx, carrier)
}

// Inject does not depend on initialization of the optional global OTel provider.
func Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	propagation.TraceContext{}.Inject(ctx, carrier)
}
