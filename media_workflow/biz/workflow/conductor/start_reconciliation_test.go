package conductor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
)

func TestFrozenStartReconciliationRejectsForeignExecution(t *testing.T) {
	for _, mutation := range []string{"none", "version", "name", "input", "routing", "identity", "correlation", "missing_input", "missing_routing", "missing_definition", "duplicates", "missing", "unavailable"} {
		t.Run(mutation, func(t *testing.T) {
			command := domain.ExecutionStart{ID: "start", JobID: "job", WorkflowName: "frozen", WorkflowVersion: 3, CorrelationID: "job", CompilationHash: strings.Repeat("a", 64), WorkflowInput: []byte(`{"job_id":"job","inputs":{"caption":"literal","count":2}}`), TaskDomains: []byte(`{"cap":"personal"}`)}
			command.PayloadDigest, _ = command.Digest()
			execution := map[string]any{"workflowId": "instance", "correlationId": "job", "workflowDefinition": map[string]any{"name": "frozen", "version": 3}, "input": map[string]any{"inputs": map[string]any{"count": 2, "caption": "literal"}, "job_id": "job"}, "taskToDomain": map[string]string{"cap": "personal"}}
			switch mutation {
			case "version":
				execution["workflowDefinition"].(map[string]any)["version"] = 4
			case "name":
				execution["workflowDefinition"].(map[string]any)["name"] = "foreign"
			case "input":
				execution["input"].(map[string]any)["extra"] = "different"
			case "routing":
				execution["taskToDomain"] = map[string]string{"cap": "foreign"}
			case "identity":
				execution["workflowId"] = "foreign"
			case "correlation":
				execution["correlationId"] = "foreign"
			case "missing_input":
				delete(execution, "input")
			case "missing_routing":
				delete(execution, "taskToDomain")
			case "missing_definition":
				delete(execution, "workflowDefinition")
			}
			var posts, reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					posts.Add(1)
					w.WriteHeader(500)
					return
				}
				reads.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if strings.Contains(r.URL.Path, "/correlated/") {
					switch mutation {
					case "missing":
						_, _ = w.Write([]byte(`[]`))
					case "duplicates":
						_, _ = w.Write([]byte(`[{"workflowId":"instance","correlationId":"job"},{"workflowId":"other","correlationId":"job"}]`))
					default:
						_, _ = w.Write([]byte(`[{"workflowId":"instance","correlationId":"job"}]`))
					}
					return
				}
				if r.URL.Path != "/api/workflow/instance" {
					t.Error("wrong exact identity")
				}
				if mutation == "unavailable" {
					w.WriteHeader(503)
					_, _ = w.Write([]byte("private upstream body"))
					return
				}
				_ = json.NewEncoder(w).Encode(execution)
			}))
			defer server.Close()
			engine := &ExecutionEngine{Client: NewClient(server.URL)}
			id, err := engine.FindStartedExecution(context.Background(), command)
			if posts.Load() != 0 {
				t.Fatal("reconciliation issued a mutation")
			}
			switch mutation {
			case "none":
				if err != nil || id != "instance" || reads.Load() != 2 {
					t.Fatal("matching frozen execution was not recovered")
				}
			case "missing":
				if err != nil || id != "" || reads.Load() != 1 {
					t.Fatal("absent result was not preserved as uncertain")
				}
			default:
				if err == nil || id != "" || strings.Contains(err.Error(), "private") {
					t.Fatal("foreign/unknown execution accepted or error exposed")
				}
			}
		})
	}
}
