package globalhertz

import (
	"context"
	"net/http"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"media_agent/hertz_infra/tracecontext"
)

func TestDisabledOTelInstallsPropagationWithoutProvider(t *testing.T) {
	before := otel.GetTracerProvider()
	previous := otel.GetTextMapPropagator()
	t.Cleanup(func() { otel.SetTextMapPropagator(previous) })
	shutdown := InitOTel(nil, nil)
	ctx, span := tracecontext.Start(context.Background(), "request", trace.SpanKindServer)
	defer span.End()
	request, err := http.NewRequestWithContext(ctx, "GET", "http://example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(request.Header))
	remote := tracecontext.Extract(context.Background(), propagation.HeaderCarrier(request.Header))
	if got := trace.SpanContextFromContext(remote); !got.IsValid() || got.TraceID() != span.SpanContext().TraceID() || got.IsSampled() {
		t.Fatal("raw HTTP adapter could not propagate an unsampled context")
	}
	if otel.GetTracerProvider() != before {
		t.Fatal("disabled OTel installed a global provider")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
