package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

type traceNativeState struct {
	ID     string         `json:"workflowId"`
	Status string         `json:"status"`
	Input  map[string]any `json:"input"`
}

func traceProbeFixture(t *testing.T) (materialsAPIFixture, string, string, func(string, string) *traceNativeState) {
	t.Helper()
	if os.Getenv("MEDIA_E2E_TRACE_PROBE") != "1" {
		t.Skip("explicit owned partial trace probe")
	}
	f := newMaterialsAPIFixture(t)
	job, engine := os.Getenv("MEDIA_E2E_TRACE_JOB_ID"), os.Getenv("MEDIA_E2E_TRACE_ENGINE_ID")
	if job == "" || engine == "" {
		t.Fatal("confirmed probe identities required")
	}
	status, data := doJSONBearer(t, f.gateway, "GET", "/api/v1/jobs/"+url.PathEscape(job), f.token, nil)
	var state struct {
		Job struct {
			ID    string `json:"id"`
			Owner string `json:"submitted_by"`
			Org   string `json:"organization_id"`
		} `json:"job"`
	}
	if status != 200 || json.Unmarshal(data, &state) != nil || state.Job.ID != job || state.Job.Owner != f.user || state.Job.Org != f.organization {
		t.Fatal("refusing unrelated probe")
	}
	native := func(method, suffix string) *traceNativeState {
		t.Helper()
		base := strings.TrimRight(envOr("MEDIA_E2E_CONDUCTOR_URL", "http://127.0.0.1:8080"), "/")
		req, _ := http.NewRequest(method, base+"/api/workflow/"+url.PathEscape(engine)+suffix, nil)
		response, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			t.Fatal("native probe operation unconfirmed")
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatal("native probe operation rejected")
		}
		if method != "GET" {
			io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			return nil
		}
		var result traceNativeState
		if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result) != nil {
			t.Fatal("native probe snapshot malformed")
		}
		return &result
	}
	n := native("GET", "")
	parent, _ := n.Input["_traceparent"].(string)
	if n.ID != engine || n.Input["job_id"] != job || !strings.HasPrefix(parent, "00-"+workflowProbeParent[3:35]+"-") {
		t.Fatal("native instance does not match frozen trace probe")
	}
	return f, job, engine, native
}

func TestWorkflowTraceProbeNativePauseResumeProjection(t *testing.T) {
	f, job, _, native := traceProbeFixture(t)
	if native("GET", "").Status != "RUNNING" {
		t.Fatal("probe is not active")
	}
	native("PUT", "/pause")
	t.Cleanup(func() {
		if native("GET", "").Status == "PAUSED" {
			native("PUT", "/resume")
		}
	})
	observe := func(want int) {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			status, data := doJSONBearer(t, f.gateway, "GET", "/api/v1/jobs/"+job, f.token, nil)
			var state struct {
				Job struct {
					Status int `json:"status"`
				} `json:"job"`
			}
			if status == 200 && json.Unmarshal(data, &state) == nil && state.Job.Status == want {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("native pause/resume did not project through public API")
	}
	observe(8)
	native("PUT", "/resume")
	observe(2)
	t.Log("native PAUSED/RUNNING projected through gateway without platform pause scheduler")
}

func TestWorkflowTraceProbeCancelAtFirstApproval(t *testing.T) {
	f, job, _, native := traceProbeFixture(t)
	deadline := time.Now().Add(60 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		status, data := doJSONBearer(t, f.gateway, "GET", "/api/v1/jobs/"+job, f.token, nil)
		var state struct {
			Job struct {
				Status int `json:"status"`
			} `json:"job"`
			Steps []struct {
				Key        string `json:"step_key"`
				Status     int    `json:"status"`
				Candidates []struct {
					URL string `json:"url"`
				} `json:"candidates"`
			} `json:"step_runs"`
		}
		if status != 200 || json.Unmarshal(data, &state) != nil {
			t.Fatal("partial probe query failed")
		}
		if state.Job.Status == 5 || state.Job.Status == 4 || state.Job.Status == 6 {
			t.Fatal("probe failed or unexpectedly finished before cancellation")
		}
		for _, step := range state.Steps {
			if step.Key == "model_image" && step.Status == 4 && len(step.Candidates) == 2 && step.Candidates[0].URL != "" && step.Candidates[1].URL != "" {
				ready = true
			}
		}
		if ready {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !ready {
		t.Fatal("first upload did not reach approval; do not approve additional nodes")
	}
	for range 2 {
		status, data := doJSONBearer(t, f.gateway, "POST", "/api/v1/jobs/"+job+"/cancel", f.token, map[string]any{})
		var result struct {
			ID     string `json:"id"`
			Status int    `json:"status"`
		}
		if status != 200 || json.Unmarshal(data, &result) != nil || result.ID != job || result.Status != 6 {
			t.Fatal("cancel intent/replay was not accepted")
		}
	}
	for time.Now().Before(deadline) {
		if native("GET", "").Status == "TERMINATED" {
			t.Log("partial upload probe cancelled; exact native instance terminated, no second complete flow")
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("accepted cancellation not confirmed by original native instance")
}
