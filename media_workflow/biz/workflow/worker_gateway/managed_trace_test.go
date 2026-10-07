package worker_gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"go.opentelemetry.io/otel/trace"
	job "media_agent/media_workflow/biz/domain/job"
)

const managedParent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"

type managedTraceReader struct{ *taskEngineStub }

func (r managedTraceReader) Get(ctx context.Context, id string) (*job.EngineExecution, error) {
	result, err := r.taskEngineStub.Get(ctx, id)
	if result != nil {
		result.Input = map[string]any{"_traceparent": managedParent}
	}
	return result, err
}

type managedTraceExecutor struct {
	t    *testing.T
	seen bool
}

func (e *managedTraceExecutor) Execute(ctx context.Context, body []byte, _ string) (map[string]any, error) {
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		e.t.Fatal("executor envelope invalid")
	}
	input := value["input"].(map[string]any)
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().TraceID().String() != managedParent[3:35] || span.IsRecording() || input["_traceparent"] != nil || input["counter"].(json.Number).String() != "9007199254740993" {
		e.t.Fatal("managed trace reached Provider data or changed numeric input")
	}
	e.seen = true
	return map[string]any{"candidates": []any{map[string]any{"content": "result"}}}, nil
}

func TestManagedRunnerRestoresTraceAndStripsItFromProviderEnvelope(t *testing.T) {
	r, flow, _, engine := newManagedFixture(t)
	flow.Executions = managedTraceReader{engine}
	engine.task.Input["counter"] = json.Number("9007199254740993")
	dispatch, err := r.Poll(context.Background(), "jimeng.image", 1)
	if err != nil {
		t.Fatal(err)
	}
	executor := &managedTraceExecutor{t: t}
	if err := r.execute(context.Background(), dispatch, executor); err != nil || !executor.seen {
		t.Fatal("managed task correlation failed", err)
	}
}
