package conductor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	"media_agent/hertz_infra/tracecontext"
)

func TestNativeAdapterPropagatesTraceWithoutCredentialsOrBaggage(t *testing.T) {
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Traceparent") != parent || r.Header.Get("Tracestate") != "" || r.Header.Get("Baggage") != "" || r.Header.Get("Authorization") != "" {
			t.Error("native request lost trace or forwarded credentials/baggage")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	ctx := tracecontext.Extract(context.Background(), propagation.MapCarrier{"traceparent": parent, "tracestate": "private=value"})
	if err := NewClient(server.URL).RetryWorkflow(ctx, "owned-fixture"); err != nil {
		t.Fatal(err)
	}
}

func TestNativePollCorrelatesButDoesNotLeakUpstreamResponse(t *testing.T) {
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Traceparent") != parent {
			t.Error("poll trace missing")
		}
		http.Error(w, "private-task-input-and-signed-url", 503)
	}))
	defer server.Close()
	ctx := tracecontext.Extract(context.Background(), propagation.MapCarrier{"traceparent": parent})
	_, err := NewClient(server.URL).Poll(ctx, "owned-task", "worker", "domain")
	if err == nil || strings.Contains(err.Error(), "private") || !strings.Contains(err.Error(), "503") {
		t.Fatal("poll error lost status or leaked upstream body")
	}
}
