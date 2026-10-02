package e2e

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

// This gate resumes one existing imported Job through the public gateway. A
// separately running media_worker must consume only media.video.generate@1.
// It never imports another lineage, creates another Job, or mocks a Worker.
func TestHistoricalResumeSingleFixture(t *testing.T) {
	if os.Getenv("MEDIA_E2E_HISTORY_RESUME") != "1" {
		t.Skip("set MEDIA_E2E_HISTORY_RESUME=1 only for the approved single sample")
	}
	addr, token, org := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_ORGANIZATION_ID")
	jobID, movementPath := os.Getenv("MEDIA_E2E_HISTORY_JOB_ID"), os.Getenv("MEDIA_E2E_HISTORY_MOVEMENT_PATH")
	videoPath := os.Getenv("MEDIA_E2E_HISTORY_VIDEO_PATH")
	if addr == "" || token == "" || org == "" || jobID == "" || movementPath == "" || videoPath == "" {
		t.Fatal("gateway, User token, Organization, existing Job, movement and fixture video Object Paths are required")
	}
	request := func(method, path string, body any, out any, want int) {
		t.Helper()
		status, response := doJSONBearer(t, addr, method, path, token, body)
		if status != want {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, path, status, want, response)
		}
		if out != nil && json.Unmarshal(response, out) != nil {
			t.Fatalf("decode %s response", path)
		}
	}
	var user struct {
		ID string `json:"id"`
	}
	request(http.MethodGet, "/api/auth/me", nil, &user, http.StatusOK)
	if user.ID == "" {
		t.Fatal("User API Token did not identify an owner")
	}
	type step struct {
		Key     string `json:"step_key"`
		Status  int    `json:"status"`
		AssetID string `json:"output_asset_id"`
	}
	type state struct {
		Job struct {
			Status int    `json:"status"`
			Owner  string `json:"submitted_by"`
			Origin string `json:"origin"`
		} `json:"job"`
		Steps []step `json:"step_runs"`
	}
	read := func() state {
		t.Helper()
		var current state
		request(http.MethodGet, "/api/v1/jobs/"+url.PathEscape(jobID), nil, &current, http.StatusOK)
		return current
	}
	before := read()
	if before.Job.Owner != user.ID || before.Job.Origin != "legacy_import" || before.Job.Status != 7 || len(before.Steps) != 4 {
		t.Fatal("sample is not an owned, awaiting-resume four-step historical Job")
	}
	frozen := map[string]string{}
	for _, item := range before.Steps {
		switch item.Key {
		case "model_image", "style_image", "first_frame":
			if item.Status != 5 || item.AssetID == "" {
				t.Fatalf("historical prefix %s is not completed", item.Key)
			}
			frozen[item.Key] = item.AssetID
		case "video":
			if item.Status != 1 || item.AssetID != "" {
				t.Fatal("video is not the sole pending suffix")
			}
		default:
			t.Fatalf("unexpected step %s", item.Key)
		}
	}
	if len(frozen) != 3 {
		t.Fatal("historical prefix is incomplete")
	}
	var object struct {
		URL string `json:"url"`
	}
	request(http.MethodGet, "/api/v1/library-objects:resolve?organization_id="+url.QueryEscape(org)+"&object_path="+url.QueryEscape(movementPath), nil, &object, http.StatusOK)
	if object.URL == "" {
		t.Fatal("movement object could not be resolved")
	}
	var videoObject struct {
		URL string `json:"url"`
	}
	request(http.MethodGet, "/api/v1/library-objects:resolve?organization_id="+url.QueryEscape(org)+"&object_path="+url.QueryEscape(videoPath), nil, &videoObject, http.StatusOK)
	if videoObject.URL == "" {
		t.Fatal("fixture video object could not be resolved")
	}
	key := "historical-e2e:" + jobID + ":1"
	request(http.MethodPost, "/api/v1/jobs/"+url.PathEscape(jobID)+"/resume", map[string]any{
		"idempotency_key": key,
		"input_patch": map[string]any{"material": []map[string]any{{
			"name": "movement", "asset_kind": "movement", "url": object.URL,
			"content": os.Getenv("MEDIA_E2E_HISTORY_MOVEMENT_CONTENT"),
		}}},
	}, nil, http.StatusAccepted)
	deadline := time.Now().Add(3 * time.Minute)
	for {
		current := read()
		if current.Job.Status == 5 || current.Job.Status == 6 {
			t.Fatalf("historical Job reached failed/cancelled status=%d", current.Job.Status)
		}
		videoAsset := ""
		for _, item := range current.Steps {
			if original, historical := frozen[item.Key]; historical && (item.Status != 5 || item.AssetID != original) {
				t.Fatalf("historical step %s was rerun or overwritten", item.Key)
			}
			if item.Key == "video" {
				videoAsset = item.AssetID
			}
		}
		if current.Job.Status == 4 {
			if videoAsset == "" {
				t.Fatal("completed Job has no video Generated Asset")
			}
			var asset struct {
				ID         string `json:"id"`
				ObjectPath string `json:"object_path"`
				URL        string `json:"url"`
			}
			request(http.MethodGet, "/api/v1/generated-assets/"+url.PathEscape(videoAsset)+"?organization_id="+url.QueryEscape(org), nil, &asset, http.StatusOK)
			if asset.ID != videoAsset || asset.ObjectPath != videoPath || asset.URL == "" {
				t.Fatalf("video Generated Asset is not backed by the expected fixture object: asset_id=%s", videoAsset)
			}
			probe, err := http.NewRequest(http.MethodGet, asset.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			probe.Header.Set("Range", "bytes=0-0")
			response, err := (&http.Client{Timeout: 10 * time.Second}).Do(probe)
			if err != nil {
				t.Fatalf("video object is not readable: %v", err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusPartialContent && response.StatusCode != http.StatusOK {
				t.Fatalf("video object returned HTTP %d", response.StatusCode)
			}
			t.Logf("historical Job resumed, only video executed: job_id=%s asset_id=%s", jobID, videoAsset)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("historical Job did not finish before deadline: job_id=%s status=%d", jobID, current.Job.Status)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
