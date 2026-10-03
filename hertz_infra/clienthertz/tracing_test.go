package clienthertz

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/hertz/pkg/protocol"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"media_agent/hertz_infra/tracecontext"
)

func TestDisabledTracingPropagatesWithoutGlobalInitialization(t *testing.T) {
	ctx := tracecontext.Extract(context.Background(), propagation.MapCarrier{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	})
	ctx, parent := tracecontext.Start(ctx, "server", trace.SpanKindServer)
	defer parent.End()
	req := protocol.AcquireRequest()
	defer protocol.ReleaseRequest(req)
	req.SetRequestURI("http://example.invalid/resource")
	req.Header.SetMethod("GET")
	req.Header.Set("Authorization", "Bearer private-credential")
	req.Header.Set("tracestate", "stale=value")
	errExpected := errors.New("upstream failed")
	endpoint := tracingMiddleware(nil)(func(child context.Context, req *protocol.Request, _ *protocol.Response) error {
		span := trace.SpanFromContext(child)
		if span.IsRecording() || span.SpanContext().IsSampled() {
			t.Fatal("local client span unexpectedly sampled")
		}
		remote := tracecontext.Extract(context.Background(), propagation.MapCarrier{
			"traceparent": string(req.Header.Peek("traceparent")),
		})
		got := trace.SpanContextFromContext(remote)
		if !got.IsValid() || got.TraceID() != parent.SpanContext().TraceID() || got.SpanID() == parent.SpanContext().SpanID() || got.SpanID() != span.SpanContext().SpanID() {
			t.Fatal("request did not carry a distinct client span in the parent's trace")
		}
		if len(req.Header.Peek("tracestate")) != 0 || string(req.Header.Peek("Authorization")) != "Bearer private-credential" {
			t.Fatal("stale trace state retained or credentials modified")
		}
		return errExpected
	})
	if err := endpoint(ctx, req, nil); !errors.Is(err, errExpected) {
		t.Fatalf("middleware lost downstream error: %v", err)
	}
}
