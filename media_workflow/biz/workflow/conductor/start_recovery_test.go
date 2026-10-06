package conductor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestStartLostResponseQueriesButNeverRepeatsStart(t *testing.T) {
	for _, result := range []string{`[{"workflowId":"existing","correlationId":"job"}]`, `[]`, `[{"workflowId":"a","correlationId":"job"},{"workflowId":"b","correlationId":"job"}]`} {
		t.Run(result, func(t *testing.T) {
			var posts, reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts.Add(1)
					w.WriteHeader(503)
					_, _ = w.Write([]byte("sensitive upstream details"))
					return
				}
				reads.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/workflow/existing" {
					_, _ = w.Write([]byte(`{"workflowId":"existing","correlationId":"job","workflowDefinition":{"name":"frozen","version":1},"input":{"job_id":"job"},"taskToDomain":{}}`))
					return
				}
				_, _ = w.Write([]byte(result))
			}))
			defer server.Close()
			engine := &ExecutionEngine{Client: NewClient(server.URL)}
			id, err := engine.Start(context.Background(), "frozen", 1, "job", map[string]any{"job_id": "job"}, nil)
			wantReads := int32(1)
			if strings.Contains(result, "existing") {
				wantReads = 2
			}
			if posts.Load() != 1 || reads.Load() != wantReads {
				t.Fatal("start replayed or omitted correlation recovery")
			}
			if strings.Contains(result, "existing") {
				if err != nil || id != "existing" {
					t.Fatal("single committed execution not recovered")
				}
			} else if err == nil || id != "" || strings.Contains(err.Error(), "sensitive") {
				t.Fatal("unknown or conflicting outcome accepted, or upstream details exposed")
			}
		})
	}
}
