package start_delivery

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
	domain "media_agent/media_workflow/biz/domain/job"
)

type tracedStartEngine struct {
	domain.ExecutionStartEngine
	parent string
	calls  int
}

func (e *tracedStartEngine) Start(ctx context.Context, _ string, _ int, _ string, input map[string]any, _ map[string]string) (string, error) {
	e.parent = trace.SpanContextFromContext(ctx).TraceID().String()
	e.calls++
	return "execution", nil
}
func (e *tracedStartEngine) FindStartedExecution(ctx context.Context, _ domain.ExecutionStart) (string, error) {
	e.parent = trace.SpanContextFromContext(ctx).TraceID().String()
	e.calls++
	return "execution", nil
}

func TestStartDeliveryRestoresFrozenTraceForLaunchAndReconciliation(t *testing.T) {
	for _, status := range []string{"queued", "uncertain"} {
		t.Run(status, func(t *testing.T) {
			input, _ := json.Marshal(map[string]any{"job_id": "job", "_traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"})
			command := &domain.ExecutionStart{ID: "start", JobID: "job", WorkflowName: "frozen", WorkflowVersion: 3, CorrelationID: "job", CompilationHash: strings.Repeat("a", 64), WorkflowInput: input, TaskDomains: []byte(`{}`), ClaimToken: "lease", Status: status}
			command.PayloadDigest, _ = command.Digest()
			var events []string
			r := &startRepo{command: command, events: &events}
			e := &tracedStartEngine{}
			f := Flow{Starts: r, Jobs: jobs{job: &domain.Job{Status: domain.JobPending, Origin: "native", CompilationHash: command.CompilationHash}}, Engine: e}
			if err := f.DispatchOne(context.Background()); err != nil || e.calls != 1 || e.parent != "4bf92f3577b34da6a3ce929d0e0e4736" || !strings.Contains(strings.Join(events, ","), "bind:execution") {
				t.Fatal("background execution lost submitted trace or failed binding", err)
			}
		})
	}
}
