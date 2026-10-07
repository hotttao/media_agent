package e2e

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

const boundaryParent = "00-10aada146cbd4b3a8d02680df358f328-990e9dfc4be9821a-00"

func TestWorkflowBoundaryProbeSubmit(t *testing.T) {
	if os.Getenv("MEDIA_E2E_BOUNDARY_PROBE") != "1" {
		t.Skip("explicit partial boundary probe")
	}
	submitOwnedPartialProbe(t, "boundary-probe-20261008-single", boundaryParent)
}

// Independent HTTP protocol gate. It never starts a model Worker, alters DB
// leases or approves candidates. Real clock expiry is deliberately observed.
func TestWorkflowNativeRerunAndLeaseBoundaryGateway(t *testing.T) {
	if os.Getenv("MEDIA_E2E_BOUNDARY_PROBE") != "1" {
		t.Skip("explicit owned boundary probe")
	}
	f := newMaterialsAPIFixture(t)
	workerToken, job := os.Getenv("MEDIA_E2E_WORKER_API_TOKEN"), os.Getenv("MEDIA_E2E_BOUNDARY_JOB_ID")
	if workerToken == "" || job == "" {
		t.Fatal("Worker token and confirmed partial Job required")
	}
	request := func(method, path, token string, body, out any) {
		t.Helper()
		status, data := doJSONBearer(t, f.gateway, method, path, token, body)
		if status != 200 || out != nil && json.Unmarshal(data, out) != nil {
			t.Fatalf("boundary operation %s status=%d; inspect original instance before retrying", path, status)
		}
	}
	var state struct {
		Job struct {
			ID    string         `json:"id"`
			Owner string         `json:"submitted_by"`
			Org   string         `json:"organization_id"`
			Input map[string]any `json:"input"`
		} `json:"job"`
	}
	request("GET", "/api/v1/jobs/"+job, f.token, nil, &state)
	if state.Job.ID != job || state.Job.Owner != f.user || state.Job.Org != f.organization {
		t.Fatal("refusing unrelated boundary Job")
	}
	instance := "boundary-e2e-20261008"
	type task struct {
		ID      string `json:"dispatch_id"`
		Attempt int    `json:"attempt"`
		Expires int64  `json:"lease_expires_at"`
		Input   string `json:"input_json"`
	}
	poll := func() task {
		t.Helper()
		var result struct {
			Task *task `json:"task"`
		}
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			status, data := doJSONBearer(t, f.gateway, "POST", "/api/worker/v1/tasks:poll", workerToken, map[string]any{"worker_instance_id": instance, "capability_key": "jimeng.i2i.multi_image_prompt", "capability_version": 1})
			if status == 204 {
				continue
			}
			if status != 200 || json.Unmarshal(data, &result) != nil || result.Task == nil {
				t.Fatal("boundary poll failed")
			}
			var input map[string]any
			if json.Unmarshal([]byte(result.Task.Input), &input) != nil || input["job_id"] != job {
				t.Fatal("another User task received; no execution performed")
			}
			return *result.Task
		}
		t.Fatal("no native task delivered")
		return task{}
	}
	first := poll()
	var receipt map[string]any
	request("POST", "/api/worker/v1/tasks/"+first.ID+"/fail", workerToken, map[string]any{"worker_instance_id": instance, "attempt": first.Attempt, "idempotency_key": "boundary:first:fail", "reason": "explicit boundary fixture failure", "failure_kind": "permanent"}, &receipt)
	deadline := time.Now().Add(15 * time.Second)
	failed := false
	for time.Now().Before(deadline) {
		var current struct {
			Job struct {
				Status int `json:"status"`
			} `json:"job"`
		}
		request("GET", "/api/v1/jobs/"+job, f.token, nil, &current)
		if current.Job.Status == 5 {
			failed = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !failed {
		t.Fatal("explicit native failure did not project")
	}
	var refresh func(any) any
	refresh = func(value any) any {
		switch v := value.(type) {
		case map[string]any:
			for key, item := range v {
				v[key] = refresh(item)
			}
		case []any:
			for n, item := range v {
				v[n] = refresh(item)
			}
		case string:
			if strings.HasPrefix(v, "object_path:") {
				var object struct {
					URL string `json:"url"`
				}
				request("GET", "/api/v1/library-objects:resolve?organization_id="+url.QueryEscape(f.organization)+"&object_path="+url.QueryEscape(strings.TrimPrefix(v, "object_path:")), f.token, nil, &object)
				if object.URL == "" {
					t.Fatal("override object unavailable")
				}
				return object.URL
			}
		}
		return value
	}
	input := refresh(state.Job.Input).(map[string]any)
	input["product"].(map[string]any)["name"] = "boundary_rerun_product_snapshot"
	request("POST", "/api/v1/jobs/"+job+"/retry", f.token, map[string]any{"reason": "explicit whole input override boundary", "input_override": input}, &receipt)
	second := poll()
	if second.ID == first.ID {
		t.Fatal("whole native rerun retained original dispatch")
	}
	var delivered map[string]any
	json.Unmarshal([]byte(second.Input), &delivered)
	parent, _ := delivered["_traceparent"].(string)
	if !strings.HasPrefix(parent, "00-"+boundaryParent[3:35]+"-") {
		t.Fatal("rerun lost original execution trace")
	}
	expire := func(expiry int64) {
		t.Helper()
		if expiry <= time.Now().Unix() || expiry-time.Now().Unix() > 100 {
			t.Fatal("unexpected local lease fixture")
		}
		target := time.Unix(expiry+2, 0)
		for time.Now().Before(target) {
			remaining := time.Until(target)
			if remaining > 15*time.Second {
				remaining = 15 * time.Second
			}
			time.Sleep(remaining)
			t.Log("observing original native Task while local metadata deadline elapses")
		}
	}
	expire(second.Expires)
	var heartbeat struct {
		Expires int64 `json:"lease_expires_at"`
	}
	request("POST", "/api/worker/v1/tasks/"+second.ID+"/heartbeat", workerToken, map[string]any{"worker_instance_id": instance, "attempt": second.Attempt}, &heartbeat)
	t.Log("heartbeat accepted after real local lease expiry")
	expire(heartbeat.Expires)
	candidates := []map[string]any{}
	for position, id := range []string{"gen_823f8029-8008-464a-b905-e37649b068ff", "gen_b1c91599-bee7-4c59-9ca6-d57ff5cbd9b5"} {
		var asset struct {
			Path string `json:"object_path"`
		}
		request("GET", "/api/v1/generated-assets/"+id+"?organization_id="+f.organization, f.token, nil, &asset)
		if !strings.HasPrefix(asset.Path, "materials/"+f.organization+"/ai_clothing/") {
			t.Fatal("known fixture output outside execution namespace")
		}
		candidates = append(candidates, map[string]any{"index": position, "object_path": asset.Path, "is_primary": position == 0})
	}
	output, _ := json.Marshal(map[string]any{"candidates": candidates, "provenance": map[string]any{"mock": true, "profile": "boundary_probe"}})
	request("POST", "/api/worker/v1/tasks/"+second.ID+"/complete", workerToken, map[string]any{"worker_instance_id": instance, "attempt": second.Attempt, "idempotency_key": "boundary:rerun:complete", "output_json": string(output)}, &receipt)
	if receipt["status"] != "submitted" {
		t.Fatal("late native completion not confirmed")
	}
	t.Log("completion accepted after real local lease expiry, same rerun Task")
	request("GET", "/api/v1/jobs/"+job, f.token, nil, &state)
	if state.Job.Input["product"].(map[string]any)["name"] != "boundary_rerun_product_snapshot" {
		t.Fatal("new input audit not committed")
	}
	request("POST", "/api/v1/jobs/"+job+"/cancel", f.token, map[string]any{}, &receipt)
	t.Log(fmt.Sprintf("partial boundary probe cancelled job_id=%s; no approval/second full flow", job))
}
