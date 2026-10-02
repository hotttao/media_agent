package conductor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	jobdomain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/workflow/compiler"
)

func TestStartResumeRegistersRestoreAndUsesConductorIdempotency(t *testing.T) {
	var taskRegistered, workflowRegistered bool
	var started StartWorkflowRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metadata/taskdefs":
			var tasks []TaskDefinition
			if err := json.NewDecoder(r.Body).Decode(&tasks); err != nil || len(tasks) != 1 || tasks[0].Name != compiler.RestoreStepTaskName {
				t.Errorf("restore Task Definition not registered: %+v err=%v", tasks, err)
			}
			taskRegistered = true
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && r.URL.Path == "/api/metadata/workflow":
			workflowRegistered = true
			_, _ = w.Write([]byte(`{"bulkErrorResults":{}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/workflow":
			if err := json.NewDecoder(r.Body).Decode(&started); err != nil {
				t.Error(err)
			}
			_, _ = w.Write([]byte(`"conductor-e2e"`))
		default:
			t.Errorf("unexpected Conductor request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	definition := compiler.Definition{Name: "resume_wf", Version: 1, SchemaVersion: 2,
		Tasks: []compiler.Task{{Name: compiler.RestoreStepTaskName, TaskReferenceName: "step__frame__finalize", Type: "SIMPLE"}}}
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	start := jobdomain.ResumeStart{JobID: "job-1", Generation: 1, CorrelationID: "resume:job-1:1",
		DefinitionJSON: encoded, TaskDomainsJSON: json.RawMessage(`{"media_video_generate_v1":"worker_user_abc"}`),
		WorkflowInputJSON: json.RawMessage(`{"job_id":"job-1","inputs":{}}`)}
	id, hash, err := NewClient(server.URL).StartResume(t.Context(), start)
	if err != nil || id != "conductor-e2e" || hash == "" || !taskRegistered || !workflowRegistered {
		t.Fatalf("resume not started: id=%q hash=%q tasks=%v workflow=%v err=%v", id, hash, taskRegistered, workflowRegistered, err)
	}
	if started.IdempotencyKey != start.CorrelationID || started.IdempotencyStrategy != "RETURN_EXISTING" ||
		started.CorrelationID != start.CorrelationID || started.TaskToDomain["media_video_generate_v1"] != "worker_user_abc" {
		t.Fatalf("Conductor idempotency/domain lost: %+v", started)
	}
}

func TestStartResumeRecoversAmbiguousStartByCorrelation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/metadata/taskdefs":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && r.URL.Path == "/api/metadata/workflow":
			_, _ = w.Write([]byte(`{"bulkErrorResults":{}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/workflow":
			w.WriteHeader(http.StatusServiceUnavailable)
		case r.Method == http.MethodGet && r.URL.Path == "/api/workflow/resume_wf/correlated/resume:job-1:1":
			_, _ = w.Write([]byte(`[{"workflowId":"existing-execution","correlationId":"resume:job-1:1"}]`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	definition := compiler.Definition{Name: "resume_wf", Version: 1, SchemaVersion: 2,
		Tasks: []compiler.Task{{Name: compiler.RestoreStepTaskName, TaskReferenceName: "restore", Type: "SIMPLE"}}}
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	start := jobdomain.ResumeStart{JobID: "job-1", CorrelationID: "resume:job-1:1", DefinitionJSON: encoded,
		TaskDomainsJSON: json.RawMessage(`{}`), WorkflowInputJSON: json.RawMessage(`{"job_id":"job-1"}`)}
	id, hash, err := NewClient(server.URL).StartResume(t.Context(), start)
	if err != nil || id != "existing-execution" || hash == "" {
		t.Fatalf("ambiguous start was not recovered: id=%q hash=%q err=%v", id, hash, err)
	}
}
