package conductor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestApprovalAndExecutionControlEndpoints(t *testing.T) {
	requests := make(chan struct {
		method string
		path   string
		body   map[string]any
	}, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests <- struct {
			method string
			path   string
			body   map[string]any
		}{r.Method, r.URL.RequestURI(), body}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx := context.Background()
	if err := client.CompleteTaskByReference(ctx, "wf/1", "step__image__approval", map[string]any{"selected_index": 1}); err != nil {
		t.Fatal(err)
	}
	if err := client.RerunFromTask(ctx, "wf/1", "task-1"); err != nil {
		t.Fatal(err)
	}
	if err := client.RerunWorkflow(ctx, "wf/1", map[string]any{"job_id": "job-1"}); err != nil {
		t.Fatal(err)
	}
	if err := client.TerminateWorkflow(ctx, "wf/1", "cancelled by user"); err != nil {
		t.Fatal(err)
	}

	complete := <-requests
	if complete.method != http.MethodPost || complete.path != "/api/tasks/wf%2F1/step__image__approval/COMPLETED?workerid=media-workflow-api" {
		t.Fatalf("complete request = %#v", complete)
	}
	rerunStep := <-requests
	if rerunStep.body["reRunFromTaskId"] != "task-1" {
		t.Fatalf("rerun step body = %#v", rerunStep.body)
	}
	rerunWorkflow := <-requests
	if rerunWorkflow.body["workflowInput"].(map[string]any)["job_id"] != "job-1" {
		t.Fatalf("rerun workflow body = %#v", rerunWorkflow.body)
	}
	terminate := <-requests
	if terminate.method != http.MethodDelete || terminate.path != "/api/workflow/wf%2F1?reason=cancelled+by+user" {
		t.Fatalf("terminate request = %#v", terminate)
	}
}
