package conductor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExecutionAdapterPreservesAuthoritativeSequencePresence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"FAILED","tasks":[{"taskId":"zero","seq":0},{"taskId":"later","seq":12},{"taskId":"missing"}]}`))
	}))
	defer server.Close()
	engine := &ExecutionEngine{Client: NewClient(server.URL)}
	execution, err := engine.Get(context.Background(), "isolated")
	if err != nil || len(execution.Tasks) != 3 {
		t.Fatal("execution adapter failed")
	}
	if execution.Tasks[0].Sequence == nil || *execution.Tasks[0].Sequence != 0 || execution.Tasks[1].Sequence == nil || *execution.Tasks[1].Sequence != 12 || execution.Tasks[2].Sequence != nil {
		t.Fatal("adapter defaulted missing sequence or lost valid zero")
	}
}
