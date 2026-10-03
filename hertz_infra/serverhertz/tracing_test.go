package serverhertz

import (
	"context"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"go.opentelemetry.io/otel/trace"

	configpb "media_agent/hertz_gen/config"
)

func TestDisabledTracingStillCorrelatesRequests(t *testing.T) {
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	for _, tc := range []struct {
		name, header, wantTrace string
	}{
		{"remote sampled parent", parent, "4bf92f3577b34da6a3ce929d0e0e4736"},
		{"no parent", "", ""},
		{"invalid parent", "not-a-trace", ""},
		{"zero parent", "00-00000000000000000000000000000000-0000000000000000-01", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := server.New()
			h.Use(newTracingMiddleware(&configpb.Config{}))
			var observed trace.SpanContext
			h.GET("/probe", func(ctx context.Context, c *app.RequestContext) {
				span := trace.SpanFromContext(ctx)
				observed = span.SpanContext()
				if span.IsRecording() || observed.IsSampled() {
					t.Error("disabled export must not record or sample, even with sampled parent")
				}
				c.Status(200)
			})
			w := ut.PerformRequest(h.Engine, "GET", "/probe", nil, ut.Header{Key: "traceparent", Value: tc.header})
			if !observed.IsValid() || observed.IsRemote() || observed.SpanID().String() == "00f067aa0ba902b7" {
				t.Fatal("handler did not receive its own valid local span")
			}
			if tc.wantTrace != "" && observed.TraceID().String() != tc.wantTrace {
				t.Fatal("upstream trace ID was not preserved")
			}
			if got := w.Header().Get("X-Trace-ID"); got != observed.TraceID().String() {
				t.Fatalf("response trace ID differs from handler context: %s", got)
			}
		})
	}
}
