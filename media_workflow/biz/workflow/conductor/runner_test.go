package conductor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"media_agent/media_workflow/biz/shared/taskcontext"
	"media_agent/media_workflow/biz/shared/taskerror"
)

func TestRunnerDelegatesRetryClassificationAndCarriesExactIdentity(t *testing.T) {
	for _, tc := range []struct {
		name           string
		err            error
		status, reason string
	}{
		{"success", nil, "COMPLETED", ""},
		{"temporary", taskerror.Retryable("storage_unavailable"), "FAILED", "storage_unavailable"},
		{"permanent", taskerror.Permanent("candidate_invalid"), "FAILED_WITH_TERMINAL_ERROR", "candidate_invalid"},
		{"unclassified", errors.New("private prompt and URL"), "FAILED", "internal_task_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			results := make(chan TaskResult, 1)
			identitySeen := make(chan bool, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					_ = json.NewEncoder(w).Encode([]Task{{TaskID: "task", WorkflowInstanceID: "workflow", TaskType: "test", ReferenceTaskName: "ref"}})
					return
				}
				var result TaskResult
				_ = json.NewDecoder(r.Body).Decode(&result)
				results <- result
				w.WriteHeader(http.StatusNoContent)
				cancel()
			}))
			defer server.Close()
			runner := Runner{Client: NewClient(server.URL), WorkerID: "internal"}
			if err := runner.runTaskType(ctx, "test", func(ctx context.Context, _ map[string]any) (map[string]any, error) {
				identity, ok := taskcontext.FromContext(ctx)
				identitySeen <- ok && identity.TaskID == "task" && identity.WorkflowID == "workflow" && identity.Reference == "ref"
				return map[string]any{"value": "result"}, tc.err
			}, time.Millisecond); !errors.Is(err, context.Canceled) {
				t.Fatal("runner did not stop", err)
			}
			result := <-results
			if !<-identitySeen || result.TaskID != "task" || result.Status != tc.status || result.ReasonForIncompletion != tc.reason || tc.err != nil && len(result.OutputData) != 0 {
				t.Fatal("runner invented retry state or leaked failed output")
			}
		})
	}
}
