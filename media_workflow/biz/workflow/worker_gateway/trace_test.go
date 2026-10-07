package worker_gateway

import (
	"context"
	"encoding/json"
	"testing"

	"media_agent/media_workflow/biz/shared/taskcontext"
)

func TestPersonalDeliveryCarriesTraceOnlyInResponseNotNativeOrStoredInput(t *testing.T) {
	f, repo, engine, snapshot, cmd := retryProjectionFixture()
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"
	snapshot.value.Input = map[string]any{taskcontext.TraceParentKey: parent}
	delivery, err := f.Poll(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	var response, stored map[string]any
	if json.Unmarshal(delivery.InputJSON, &response) != nil || json.Unmarshal(repo.dispatch.InputJSON, &stored) != nil || response[taskcontext.TraceParentKey] != parent || stored[taskcontext.TraceParentKey] != nil || engine.task.Input[taskcontext.TraceParentKey] != nil {
		t.Fatal("delivery metadata missing or persisted into business task")
	}
	// A broken/missing trace must not fail delivery or bypass task identity.
	snapshot.value.Input[taskcontext.TraceParentKey] = "private-invalid-trace"
	repo.active = repo.dispatch
	delivery, err = f.Poll(context.Background(), cmd)
	if err != nil || json.Unmarshal(delivery.InputJSON, &response) != nil {
		t.Fatal("invalid optional metadata blocked task", err)
	}
	var clean map[string]any
	json.Unmarshal(delivery.InputJSON, &clean)
	if clean[taskcontext.TraceParentKey] != nil {
		t.Fatal("invalid trace exposed")
	}
}
