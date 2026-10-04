// Package delegationaudit records request-level delegation evidence, not a
// transactional business ledger. Its fixed schema cannot carry HTTP payloads.
package delegationaudit

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"go.opentelemetry.io/otel/trace"
	"media_agent/hertz_infra/authn/internaljwt"
)

type Event struct {
	Event          string `json:"event"`
	Phase          string `json:"phase"`
	Stage          string `json:"stage"`
	Outcome        string `json:"outcome"`
	Service        string `json:"service"`
	Method         string `json:"method"`
	RouteTemplate  string `json:"route_template"`
	StatusCode     int    `json:"status_code"`
	DurationMS     int64  `json:"duration_ms"`
	UserID         string `json:"user_id,omitempty"`
	ActorID        string `json:"actor_id,omitempty"`
	CredentialID   string `json:"credential_id,omitempty"`
	OrganizationID string `json:"organization_id,omitempty"`
	WorkspaceID    string `json:"workspace_id,omitempty"`
	TraceID        string `json:"trace_id,omitempty"`
	ResultUnknown  bool   `json:"result_unknown"`
}

func (e *Event) SetPrincipal(p internaljwt.AgentPrincipal) {
	e.UserID, e.ActorID, e.CredentialID, e.OrganizationID, e.WorkspaceID = p.UserID, p.ActorID, p.CredentialID, p.OrganizationID, p.WorkspaceID
}

type Recorder interface{ Record(context.Context, Event) }

type LogRecorder struct{}

func (LogRecorder) Record(ctx context.Context, e Event) {
	encoded := Encode(ctx, e)
	hlog.CtxInfof(ctx, "%s", encoded)
}

// Encode attaches the actual context trace, never a caller-supplied trace ID.
// Identifiers and templates are bounded and cannot contain query/header text.
func Encode(ctx context.Context, e Event) []byte {
	e.TraceID = ""
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		e.TraceID = span.TraceID().String()
	}
	for _, value := range []*string{&e.UserID, &e.ActorID, &e.CredentialID, &e.OrganizationID, &e.WorkspaceID} {
		if !safeText(*value, 160, "_:-") {
			*value = ""
		}
	}
	if !safeText(e.RouteTemplate, 256, "/_:*-") {
		e.RouteTemplate = ""
	}
	for _, value := range []*string{&e.Event, &e.Phase, &e.Stage, &e.Outcome, &e.Service, &e.Method} {
		if !safeText(*value, 64, "_-") {
			*value = ""
		}
	}
	encoded, _ := json.Marshal(e) // fixed scalar schema: Marshal cannot fail
	return encoded
}

func safeText(s string, limit int, punctuation string) bool {
	if len(s) > limit {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune(punctuation, c)) {
			return false
		}
	}
	return true
}

// BusinessOutcome is deferred immediately after request-bound JWT validation,
// so later membership/resource denials retain the verified calling identity.
func BusinessOutcome(ctx context.Context, p internaljwt.AgentPrincipal, service, method, route string, status func() int, recorders ...Recorder) func() {
	started := time.Now()
	var recorder Recorder = LogRecorder{}
	if len(recorders) > 0 && recorders[0] != nil {
		recorder = recorders[0]
	}
	return func() {
		panicValue := recover()
		e := Event{Event: "agent_business_request", Phase: "business_service", Stage: "resource_response", Service: service,
			Method: method, RouteTemplate: route, StatusCode: status(), DurationMS: time.Since(started).Milliseconds()}
		e.SetPrincipal(p)
		e.Outcome = ResponseOutcome(e.StatusCode)
		if panicValue != nil {
			e.StatusCode, e.Outcome, e.ResultUnknown = 500, "panic", true
		}
		recorder.Record(ctx, e)
		if panicValue != nil {
			panic(panicValue)
		}
	}
}

func ResponseOutcome(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "success"
	case status >= 300 && status < 400:
		return "redirect_blocked"
	case status >= 400 && status < 500:
		return "rejected"
	default:
		return "failed"
	}
}
