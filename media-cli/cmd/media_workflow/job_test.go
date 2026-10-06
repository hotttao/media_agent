package media_workflow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	jobsvc "media_agent/hertz_gen/media_workflow/job_service"
)

func TestStepRetryCommandSendsOnceAndDoesNotExposePayload(t *testing.T) {
	for _, status := range []int{200, 400, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil || r.Method != "POST" || r.URL.Path != "/api/v1/step-runs/step/retry" || body["reason"] != "test retry" {
					t.Error("retry request mapping incorrect")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"id":"step","status":1,"candidates":[{"url":"https://storage.example?signature=private"}],"rendered_prompt":"private-prompt"}`))
			}))
			defer server.Close()
			old := JobCli
			defer func() { JobCli = old }()
			var err error
			JobCli, err = jobsvc.NewJobServiceClient(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			command := newStepRunCommand()
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(&output)
			command.SetArgs([]string{"retry", "--id", "step", "--reason", "test retry"})
			err = command.Execute()
			if (err == nil) != (status == 200) || calls.Load() != 1 {
				t.Fatal("retry outcome wrong or request repeated")
			}
			if strings.Contains(output.String(), "signature") || strings.Contains(output.String(), "private-prompt") {
				t.Fatal("sensitive retry response printed")
			}
			if err != nil && !strings.Contains(err.Error(), "query execution") {
				t.Fatal("ambiguous command lacks recovery guidance")
			}
		})
	}
}

func TestReviewCommandsUseStableKeysAndSafeReceipts(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		requests.Add(1)
		if body["idempotency_key"] != "stable" || r.Method != "POST" {
			t.Error("lost review identity")
		}
		if strings.HasSuffix(r.URL.Path, "/confirm") && body["selected_index"] != float64(1) {
			t.Error("selection was not array position")
		}
		if strings.HasSuffix(r.URL.Path, "/regenerate") && body["feedback"] != "更亮 {{.literal}}" {
			t.Error("feedback lost")
		}
		w.WriteHeader(202)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "step", "step_key": "image", "status": 3, "last_review_id": "review", "last_review_status": "pending", "candidates": []map[string]any{{"url": "https://storage.example/private?signature=secret"}}, "rendered_prompt": "private-prompt"})
	}))
	defer server.Close()
	old := JobCli
	defer func() { JobCli = old }()
	var err error
	JobCli, err = jobsvc.NewJobServiceClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"confirm", "--id", "step", "--selected-index", "1", "--idempotency-key", "stable"},
		{"regenerate", "--id", "step", "--feedback", "更亮 {{.literal}}", "--idempotency-key", "stable"},
	} {
		command := newStepRunCommand()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), `"last_review_id":"review"`) || strings.Contains(output.String(), "signature") || strings.Contains(output.String(), "private-prompt") {
			t.Fatal("receipt missing or sensitive payload printed")
		}
	}
	if requests.Load() != 2 {
		t.Fatal("command repeated request")
	}
}

func TestReviewCommandRejectsMissingKeyWithoutSending(t *testing.T) {
	command := newStepRunCommand()
	command.SetArgs([]string{"confirm", "--id", "step", "--selected-index", "0"})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	if err := command.Execute(); err == nil {
		t.Fatal("review accepted without caller-stable key")
	}
}

func TestReviewCommandDoesNotExposeUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		_, _ = w.Write([]byte("secret upstream response https://storage.example?signature=private"))
	}))
	defer server.Close()
	old := JobCli
	defer func() { JobCli = old }()
	var err error
	JobCli, err = jobsvc.NewJobServiceClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	command := newStepRunCommand()
	command.SetArgs([]string{"confirm", "--id", "step", "--selected-index", "0", "--idempotency-key", "stable"})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	err = command.Execute()
	if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "signature") {
		t.Fatal("upstream failure accepted or exposed")
	}
}
