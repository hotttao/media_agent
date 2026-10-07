package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

const workflowProbeParent = "00-6f47731a6d414c85958022531982a167-108ce5b6afd746f1-00"

// Create one partial, explicitly keyed probe. Do not run a Worker or approve
// any node here; the operator later observes uploads then cancels this Job.
func TestWorkflowTraceProbeSubmit(t *testing.T) {
	if os.Getenv("MEDIA_E2E_TRACE_PROBE") != "1" {
		t.Skip("explicit partial trace probe")
	}
	submitOwnedPartialProbe(t, "trace-probe-20261007-single", workflowProbeParent)
}

func submitOwnedPartialProbe(t *testing.T, intent, parent string) {
	t.Helper()
	f := newMaterialsAPIFixture(t)
	source := os.Getenv("MEDIA_E2E_TRACE_SOURCE_JOB_ID")
	if source == "" {
		t.Fatal("owned completed source sample required")
	}
	read := func(path string, out any) {
		status, data := doJSONBearer(t, f.gateway, "GET", path, f.token, nil)
		if status != 200 || json.Unmarshal(data, out) != nil {
			t.Fatal("trace probe preflight unavailable")
		}
	}
	// Explicit wire fields, never print the private input or URLs.
	var sourceState struct {
		Job struct {
			ID           string         `json:"id"`
			Owner        string         `json:"submitted_by"`
			Organization string         `json:"organization_id"`
			Workspace    string         `json:"workspace_id"`
			Workflow     string         `json:"workflow_id"`
			Version      int            `json:"workflow_version"`
			Status       int            `json:"status"`
			Input        map[string]any `json:"input"`
		} `json:"job"`
	}
	read("/api/v1/jobs/"+url.PathEscape(source), &sourceState)
	j := sourceState.Job
	if j.ID != source || j.Owner != f.user || j.Organization != f.organization || j.Workspace == "" || j.Status != 4 {
		t.Fatal("source is not an owned completed sample")
	}
	for page := 1; page <= 1000; page++ {
		var list struct {
			Total int `json:"total"`
			Items []struct {
				Status int `json:"status"`
			} `json:"items"`
		}
		read("/api/v1/jobs?page_size=200&page="+strconv.Itoa(page), &list)
		for _, item := range list.Items {
			if item.Status == 1 || item.Status == 2 || item.Status == 3 || item.Status == 8 {
				t.Fatal("another active User Job exists; refuse trace probe")
			}
		}
		if page*200 >= list.Total {
			break
		}
		if len(list.Items) == 0 || page == 1000 {
			t.Fatal("queue audit incomplete")
		}
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
				path := strings.TrimPrefix(v, "object_path:")
				var object struct {
					URL string `json:"url"`
				}
				read("/api/v1/library-objects:resolve?organization_id="+url.QueryEscape(f.organization)+"&object_path="+url.QueryEscape(path), &object)
				if object.URL == "" {
					t.Fatal("probe input object unavailable")
				}
				return object.URL
			}
		}
		return value
	}
	input := refresh(j.Input)
	body, _ := json.Marshal(map[string]any{"organization_id": j.Organization, "workspace_id": j.Workspace, "workflow_id": j.Workflow, "workflow_version": j.Version, "input": input, "idempotency_key": intent})
	req, err := http.NewRequest("POST", f.gateway+"/api/v1/jobs", bytes.NewReader(body))
	if err != nil {
		t.Fatal("probe request invalid")
	}
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("traceparent", parent)
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal("probe Submit outcome unknown; query fixed intent before retrying")
	}
	defer response.Body.Close()
	var accepted struct {
		ID string `json:"job_id"`
	}
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&accepted) != nil || accepted.ID == "" {
		t.Fatal("probe Submit not acknowledged; do not create another intent")
	}
	if response.Header.Get("X-Trace-ID") != parent[3:35] {
		t.Fatal("gateway/request trace did not survive to response; accepted Job must be inspected")
	}
	t.Log("partial probe accepted job_id=" + accepted.ID + " trace_id=" + parent[3:35])
}
