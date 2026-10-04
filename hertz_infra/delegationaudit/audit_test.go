package delegationaudit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/tracecontext"
)

type auditCapture struct{ events []Event }

func (s *auditCapture) Record(_ context.Context, e Event) { s.events = append(s.events, e) }

func TestAuditEncodingBoundsMetadataAndUsesActualTrace(t *testing.T) {
	ctx, span := tracecontext.Start(t.Context(), "audit-test", trace.SpanKindServer)
	defer span.End()
	e := Event{Event: "agent_proxy_request", Phase: "gateway", Outcome: "success", TraceID: "forged", StatusCode: 200,
		RouteTemplate: "/api/v1/materials?token=private-query", UserID: "owner\nsecret", ActorID: "Agent:config", CredentialID: "manager-id", WorkspaceID: strings.Repeat("x", 161)}
	encoded := Encode(ctx, e)
	var got Event
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.TraceID != span.SpanContext().TraceID().String() || got.ActorID != e.ActorID || got.CredentialID != e.CredentialID || got.UserID != "" || got.WorkspaceID != "" || got.RouteTemplate != "" {
		t.Fatal("untrusted metadata or trace entered audit")
	}
	for _, forbidden := range []string{"forged", "private-query", "secret"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("unsafe audit content")
		}
	}
}

func TestBusinessAuditObservesFinalStatusAndVerifiedIdentity(t *testing.T) {
	for _, tc := range []struct {
		status  int
		outcome string
	}{{200, "success"}, {201, "success"}, {403, "rejected"}, {503, "failed"}} {
		recorder := &auditCapture{}
		status := 200
		p := internaljwt.AgentPrincipal{UserID: "owner", ActorID: "Agent:config", CredentialID: "credential", OrganizationID: "org", WorkspaceID: "workspace"}
		finish := BusinessOutcome(t.Context(), p, "materials", "PUT", "/api/v1/products/:id", func() int { return status }, recorder)
		status = tc.status
		finish()
		if len(recorder.events) != 1 {
			t.Fatal("missing terminal business audit")
		}
		got := recorder.events[0]
		if got.StatusCode != status || got.Outcome != tc.outcome || got.UserID != p.UserID || got.ActorID != p.ActorID || got.CredentialID != p.CredentialID || got.WorkspaceID != p.WorkspaceID || got.OrganizationID != p.OrganizationID || got.ResultUnknown || got.RouteTemplate != "/api/v1/products/:id" {
			t.Fatal("business audit lost identity or final response")
		}
	}
}

func TestBusinessAuditDoesNotMisreportPanicAsSuccessOrSwallowIt(t *testing.T) {
	sink := &auditCapture{}
	func() {
		defer func() {
			if recover() != "private-panic-body" {
				t.Fatal("panic changed or swallowed")
			}
		}()
		defer BusinessOutcome(t.Context(), internaljwt.AgentPrincipal{UserID: "owner", ActorID: "Agent:config"}, "workflow", "POST", "/api/v1/jobs", func() int { return 200 }, sink)()
		panic("private-panic-body")
	}()
	if len(sink.events) != 1 || sink.events[0].StatusCode != 500 || sink.events[0].Outcome != "panic" || !sink.events[0].ResultUnknown || strings.Contains(string(Encode(t.Context(), sink.events[0])), "private-panic-body") {
		t.Fatal("panic was recorded as success or leaked")
	}
}
