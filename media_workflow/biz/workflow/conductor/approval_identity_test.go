package conductor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestApprovalSubmissionUsesFrozenTaskIDWithoutReferenceLookup(t *testing.T) {
	var calls atomic.Int32
	var invalid atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var result TaskResult
		if r.Method != "POST" || r.URL.Path != "/api/tasks" || json.NewDecoder(r.Body).Decode(&result) != nil || result.WorkflowInstanceID != "execution" || result.TaskID != "frozen-human" || result.Status != "COMPLETED" || result.OutputData["decision"] != "accept" {
			invalid.Store(true)
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	engine := &ExecutionEngine{Client: NewClient(server.URL)}
	if engine.CompleteApprovalTask(context.Background(), "execution", "frozen-human", map[string]any{"decision": "accept"}) != nil || calls.Load() != 1 || invalid.Load() {
		t.Fatal("approval identity was redirected or resolved by reference")
	}
	if engine.CompleteApprovalTask(context.Background(), "execution", "", nil) == nil || calls.Load() != 1 {
		t.Fatal("empty identity submitted")
	}
}
