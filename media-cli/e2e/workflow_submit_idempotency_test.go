package e2e

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Explicit cancellation-only sample: eight requests represent one intent.
// No fixture Worker or model provider is started by this gate.
func TestWorkflowSubmitIdempotencyGateway(t *testing.T) {
	if os.Getenv("MEDIA_E2E_SUBMIT_IDEMPOTENCY_SAMPLE") != "1" {
		t.Skip("explicit submission sample gate")
	}
	address, token, source := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_SUBMIT_SOURCE_JOB_ID")
	if address == "" || token == "" || source == "" {
		t.Fatal("gateway, User token and source fixture required")
	}
	read := func(path string, out any) {
		t.Helper()
		status, data := doJSONBearer(t, address, "GET", path, token, nil)
		if status != 200 || json.Unmarshal(data, out) != nil {
			t.Fatal("authorized fixture read failed")
		}
	}
	var snapshot struct {
		Job struct {
			Workflow     string         `json:"workflow_id"`
			Version      int            `json:"workflow_version"`
			Organization string         `json:"organization_id"`
			Workspace    string         `json:"workspace_id"`
			Input        map[string]any `json:"input"`
		} `json:"job"`
	}
	read("/api/v1/jobs/"+source, &snapshot)
	j := snapshot.Job
	if j.Input == nil || j.Organization == "" || j.Workspace == "" || j.Workflow == "" {
		t.Fatal("source fixture missing authorized input")
	}
	var resolve func(any)
	resolve = func(value any) {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				resolve(item)
			}
		case map[string]any:
			for key, item := range typed {
				if raw, ok := item.(string); ok && (key == "url" || strings.HasSuffix(key, "_url")) && strings.HasPrefix(raw, "object_path:") {
					var object struct {
						URL string `json:"url"`
					}
					read("/api/v1/library-objects:resolve?organization_id="+url.QueryEscape(j.Organization)+"&object_path="+url.QueryEscape(strings.TrimPrefix(raw, "object_path:")), &object)
					if object.URL == "" {
						t.Fatal("fixture media resolution failed")
					}
					typed[key] = object.URL
				} else {
					resolve(item)
				}
			}
		}
	}
	resolve(j.Input)
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal("test intent identity unavailable")
	}
	payload := map[string]any{"workflow_id": j.Workflow, "workflow_version": j.Version, "workspace_id": j.Workspace, "organization_id": j.Organization, "input": j.Input, "idempotency_key": "http-submit:" + hex.EncodeToString(nonce[:])}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var ids []string
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, data := doJSONBearer(t, address, "POST", "/api/v1/jobs", token, payload)
			var result struct {
				ID string `json:"job_id"`
			}
			if status != 200 || json.Unmarshal(data, &result) != nil || result.ID == "" {
				t.Error("concurrent intent not accepted")
				return
			}
			mu.Lock()
			ids = append(ids, result.ID)
			mu.Unlock()
		}()
	}
	wg.Wait()
	// Clean up only acknowledged sample IDs, including an erroneous duplicate.
	unique := map[string]bool{}
	for _, id := range ids {
		unique[id] = true
	}
	t.Cleanup(func() {
		for id := range unique {
			status, _ := doJSONBearer(t, address, "POST", "/api/v1/jobs/"+id+"/cancel", token, map[string]any{})
			if status != 200 {
				t.Error("sample cancellation cleanup failed")
			}
		}
	})
	if len(ids) != 8 || len(unique) != 1 {
		t.Fatal("same intent did not return exactly one Job")
	}
	id := ids[0]
	t.Log("single submission sample job_id=" + id)
	for deadline := time.Now().Add(20 * time.Second); ; {
		var state struct {
			Job struct {
				Status int `json:"status"`
			} `json:"job"`
		}
		read("/api/v1/jobs/"+id, &state)
		if state.Job.Status == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("accepted sample did not start; do not resubmit with a new key")
		}
		time.Sleep(200 * time.Millisecond)
	}
	status, _ := doJSONBearer(t, address, "POST", "/api/v1/jobs/"+id+"/cancel", token, map[string]any{})
	if status != 200 {
		t.Fatal("accepted sample cancellation failed")
	}
	// Poison only transport signatures. Replaying must not fetch these URLs.
	var poison func(any)
	poison = func(value any) {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				poison(item)
			}
		case map[string]any:
			for key, item := range typed {
				if raw, ok := item.(string); ok && (key == "url" || strings.HasSuffix(key, "_url")) && raw != "" {
					u, err := url.Parse(raw)
					if err != nil {
						t.Fatal("invalid fixture URL")
					}
					q := u.Query()
					q.Set("signature", "intentionally-invalid-replay-signature")
					u.RawQuery = q.Encode()
					typed[key] = u.String()
				} else {
					poison(item)
				}
			}
		}
	}
	poison(j.Input)
	status, data := doJSONBearer(t, address, "POST", "/api/v1/jobs", token, payload)
	var replay struct {
		ID     string `json:"job_id"`
		Status int    `json:"status"`
	}
	if status != 200 || json.Unmarshal(data, &replay) != nil || replay.ID != id || replay.Status != 6 {
		t.Fatal("terminal replay did not return original cancelled Job")
	}
	payload["workflow_version"] = j.Version + 100
	status, _ = doJSONBearer(t, address, "POST", "/api/v1/jobs", token, payload)
	if status != 409 {
		t.Fatal("same key with changed version was not rejected")
	}
}
