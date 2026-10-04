package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/delegationaudit"
	"media_agent/hertz_infra/tracecontext"
	"media_agent/media_agent_gateway/biz/shared/client"
)

type proxyAuditCapture struct {
	events  []delegationaudit.Event
	encoded []byte
}

func (s *proxyAuditCapture) Record(ctx context.Context, e delegationaudit.Event) {
	s.events = append(s.events, e)
	s.encoded = delegationaudit.Encode(ctx, e)
}

type auditForward func(context.Context, AgentProxyRequest, string) (AgentProxyResponse, error)

func (f auditForward) ForwardAgentRequest(ctx context.Context, r AgentProxyRequest, s string) (AgentProxyResponse, error) {
	return f(ctx, r, s)
}

func TestAgentProxyAuditStagesOnlyUseVerifiedIdentity(t *testing.T) {
	for _, tc := range []struct {
		name                string
		change              func(*proxyFixture, *AgentProxyRequest)
		stage               string
		status              int
		identity, workspace bool
	}{
		{"invalid operation", func(_ *proxyFixture, r *AgentProxyRequest) { r.Request.URI = "/internal/secret?token=private-query" }, "request_validation", 400, false, false},
		{"rejected credential", func(f *proxyFixture, _ *AgentProxyRequest) { f.verifyErr = client.ErrAgentCredentialRejected }, "credential_verification", 401, false, false},
		{"authority denied", func(f *proxyFixture, _ *AgentProxyRequest) { f.facts.OwnerAuthorized = false }, "authority_lookup", 403, true, false},
		{"success", func(*proxyFixture, *AgentProxyRequest) {}, "business_response", 200, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newProxyFixture()
			sink := &proxyAuditCapture{}
			r := AgentProxyRequest{Service: "workflow", Request: internaljwt.AgentRequest{Method: "GET", URI: "/api/v1/jobs/resource-secret?token=private-query", Body: []byte("private-body"), ContentType: "application/json"}}
			tc.change(f, &r)
			ctx, span := tracecontext.Start(t.Context(), "proxy-audit", trace.SpanKindServer)
			defer span.End()
			_, _ = NewAgentAPIProxyWithAudit(f, f, f, f, sink).Execute(ctx, "private-api-token", r)
			if len(sink.events) != 1 {
				t.Fatal("missing or duplicate audit")
			}
			got := sink.events[0]
			if got.Stage != tc.stage || got.StatusCode != tc.status || (got.ActorID != "") != tc.identity || (got.WorkspaceID != "") != tc.workspace {
				t.Fatal("incorrect identity stage")
			}
			if tc.identity && (got.UserID != "owner" || got.CredentialID != "credential" || got.OrganizationID != "org") {
				t.Fatal("identity lost")
			}
			if tc.workspace && (got.WorkspaceID != "workspace" || got.RouteTemplate != "/api/v1/jobs/:id") {
				t.Fatal("untrusted workspace or resource ID logged")
			}
			var serialized delegationaudit.Event
			if err := json.Unmarshal(sink.encoded, &serialized); err != nil || serialized.TraceID != span.SpanContext().TraceID().String() {
				t.Fatal("context trace lost")
			}
			for _, s := range []string{"private-api-token", "private-query", "private-body", "resource-secret", "internal-assertion", "untrusted"} {
				if strings.Contains(string(sink.encoded), s) {
					t.Fatal("secret/request leaked into audit")
				}
			}
		})
	}
}

func TestAgentProxyAuditDoesNotMistakeBusinessFailureOrTimeoutForSuccess(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		cancel  bool
		err     error
		outcome string
		unknown bool
	}{
		{"business forbidden", 403, false, nil, "rejected", false},
		{"business unavailable", 503, false, nil, "failed", false},
		{"result lost", 0, false, errors.New("sensitive-error-body"), "upstream_unavailable", true},
		{"cancelled after forwarding", 0, true, context.Canceled, "cancelled", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			f := newProxyFixture()
			sink := &proxyAuditCapture{}
			calls := 0
			forward := auditForward(func(context.Context, AgentProxyRequest, string) (AgentProxyResponse, error) {
				calls++
				if tc.cancel {
					cancel()
				}
				return AgentProxyResponse{StatusCode: tc.status}, tc.err
			})
			_, _ = NewAgentAPIProxyWithAudit(f, f, f, forward, sink).Execute(ctx, "secret", AgentProxyRequest{Service: "materials", Request: internaljwt.AgentRequest{Method: "GET", URI: "/api/v1/materials"}})
			if calls != 1 || len(sink.events) != 1 || sink.events[0].Outcome != tc.outcome || sink.events[0].ResultUnknown != tc.unknown || strings.Contains(string(sink.encoded), "sensitive-error-body") {
				t.Fatal("result uncertainty lost or write replayed")
			}
			want := tc.status
			if tc.err != nil {
				want = 503
			}
			if sink.events[0].StatusCode != want {
				t.Fatal("envelope status replaced business result")
			}
		})
	}
}

func TestAgentProxyAuditRecordsPanicWithoutLeakingOrSwallowingIt(t *testing.T) {
	f := newProxyFixture()
	sink := &proxyAuditCapture{}
	forward := auditForward(func(context.Context, AgentProxyRequest, string) (AgentProxyResponse, error) {
		panic("private-panic-body")
	})
	func() {
		defer func() {
			if recover() != "private-panic-body" {
				t.Fatal("panic changed or swallowed")
			}
		}()
		_, _ = NewAgentAPIProxyWithAudit(f, f, f, forward, sink).Execute(t.Context(), "private-token", AgentProxyRequest{Service: "workflow", Request: internaljwt.AgentRequest{Method: "GET", URI: "/api/v1/jobs"}})
	}()
	if len(sink.events) != 1 || sink.events[0].Outcome != "panic" || sink.events[0].StatusCode != 500 || !sink.events[0].ResultUnknown || strings.Contains(string(sink.encoded), "private-panic-body") {
		t.Fatal("panic result was misreported or leaked")
	}
}
