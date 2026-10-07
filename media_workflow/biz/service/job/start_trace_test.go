package job

import (
	"context"
	"encoding/json"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	"media_agent/hertz_infra/tracecontext"
	"media_agent/media_workflow/biz/shared/taskcontext"
)

func TestSubmitTraceIsFrozenButNotPartOfSubmissionReplayIdentity(t *testing.T) {
	s, jobs, starts, ledger, _, cmd := submissionFixture()
	s.resumeURLVerifier = func(context.Context, string) error { return nil }
	firstParent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	ctx := tracecontext.Extract(context.Background(), propagation.MapCarrier{"traceparent": firstParent})
	first, err := s.Submit(ctx, cmd)
	if err != nil || starts.command == nil {
		t.Fatal("traced Submit failed", err)
	}
	var input map[string]any
	if json.Unmarshal(starts.command.WorkflowInput, &input) != nil || input[taskcontext.TraceParentKey] != firstParent || starts.command.Validate() != nil {
		t.Fatal("trace missing from validated frozen Start")
	}
	frozen := string(starts.command.WorkflowInput)
	ctx = tracecontext.Extract(context.Background(), propagation.MapCarrier{"traceparent": "00-11111111111111111111111111111111-2222222222222222-00"})
	replay, err := s.Submit(ctx, cmd)
	if err != nil || replay.ID != first.ID || len(jobs.jobs) != 1 || ledger.writes != 1 || string(starts.command.WorkflowInput) != frozen {
		t.Fatal("new request trace changed submission or frozen parent", err)
	}
}
