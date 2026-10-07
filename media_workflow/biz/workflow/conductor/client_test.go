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

func TestNativeRetryUsesEngineEndpointWithoutResetPayload(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.RequestURI() != "/api/workflow/execution%2F1/retry" || r.ContentLength != 0 {
			t.Error("native retry changed execution identity or supplied reset payload")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := NewClient(server.URL)
	if err := client.RetryWorkflow(t.Context(), "execution/1"); err != nil {
		t.Fatal(err)
	}
	if client.RetryWorkflow(t.Context(), "") == nil || calls != 1 {
		t.Fatal("missing identity reached native retry endpoint")
	}
}

func TestStartWorkflowUsesRequestBodyAndTaskDomain(t *testing.T) {
	var got StartWorkflowRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/workflow" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`"workflow-1"`))
	}))
	defer server.Close()

	id, err := NewClient(server.URL).StartWorkflow(t.Context(), StartWorkflowRequest{
		Name: "compiled", Version: 3, CorrelationID: "job-1",
		Input:        map[string]any{"job_id": "job-1"},
		TaskToDomain: map[string]string{"media_image_model_v1": "worker_user_abc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "workflow-1" || got.CorrelationID != "job-1" || got.TaskToDomain["media_image_model_v1"] != "worker_user_abc" {
		t.Fatalf("id=%q request=%+v", id, got)
	}
}

func TestPollIncludesExactDomainWithoutFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("domain"); got != "worker_user_abc" {
			t.Fatalf("domain=%q", got)
		}
		if r.URL.Path != "/api/tasks/poll/batch/media_image_model_v1" || r.URL.Query().Get("timeout") != "5000" || r.URL.Query().Get("count") != "1" {
			t.Fatalf("request=%s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()
	task, err := NewClient(server.URL).Poll(t.Context(), "media_image_model_v1", "proxy", "worker_user_abc")
	if err != nil || task != nil {
		t.Fatalf("task=%v err=%v", task, err)
	}
}

func TestStartWorkflowAcceptsRawOSSUUIDButRejectsMalformedBody(t *testing.T) {
	for _, body := range []string{"01234567-89ab-4cde-8123-0123456789ab", "{\"error\":\"invalid\"}", "", "123"} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			id, err := NewClient(server.URL).StartWorkflow(t.Context(), StartWorkflowRequest{Name: "test", Version: 1})
			if body == "01234567-89ab-4cde-8123-0123456789ab" {
				if err != nil || id != body {
					t.Fatalf("raw UUID: id=%q err=%v", id, err)
				}
			} else if err == nil {
				t.Fatal("malformed workflow id accepted")
			}
		})
	}
}

func TestPollReturnsFirstBatchTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]Task{{TaskID: "task-1", WorkflowInstanceID: "workflow-1", RetryCount: 2}})
	}))
	defer server.Close()
	task, err := NewClient(server.URL).Poll(t.Context(), "media_image_model_v1", "proxy", "worker_user_abc")
	if err != nil || task == nil || task.TaskID != "task-1" || task.RetryCount != 2 {
		t.Fatalf("task=%#v err=%v", task, err)
	}
}
