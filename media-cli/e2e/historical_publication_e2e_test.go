package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// Read-only gate against an imported source record. It checks real member Jobs
// and streams target media to verify the persisted checksum, without workers.
func TestHistoricalPublicationGateway(t *testing.T) {
	id := os.Getenv("MEDIA_E2E_HISTORICAL_PUBLICATION_ID")
	if id == "" {
		t.Skip("set MEDIA_E2E_HISTORICAL_PUBLICATION_ID to an imported sample")
	}
	addr, token, workspace := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_WORKSPACE_ID")
	if addr == "" || token == "" || workspace == "" {
		t.Fatal("gateway, User API Token and Workspace are required")
	}
	type media struct {
		ObjectPath string `json:"object_path"`
		SHA256     string `json:"sha256"`
		URL        string `json:"access_url"`
	}
	type member struct {
		Ordinal  int    `json:"ordinal"`
		SourceID string `json:"source_video_id"`
		JobID    string `json:"job_id"`
	}
	type push struct {
		ID        string   `json:"id"`
		Workspace string   `json:"workspace_id"`
		SourceID  string   `json:"source_record_id"`
		Members   []member `json:"members"`
		Video     media    `json:"video"`
		Thumbnail media    `json:"thumbnail"`
		Manual    media    `json:"manual_clip"`
	}
	var response struct {
		Item push `json:"item"`
	}
	path := "/api/v1/historical-video-pushes/" + url.PathEscape(id)
	status, body := doJSONBearer(t, addr, http.MethodGet, path, token, nil)
	if status != http.StatusOK || json.Unmarshal(body, &response) != nil {
		t.Fatalf("read historical publication: HTTP %d", status)
	}
	item := response.Item
	if item.ID != id || item.Workspace != workspace || item.SourceID == "" || len(item.Members) == 0 {
		t.Fatal("publication identity or members missing")
	}
	seen := map[string]bool{}
	for i, m := range item.Members {
		if m.Ordinal != i || m.SourceID == "" || m.JobID == "" || seen[m.SourceID] {
			t.Fatal("invalid publication member ordering")
		}
		seen[m.SourceID] = true
		var job struct {
			Job struct {
				Status    int    `json:"status"`
				SourceID  string `json:"source_record_id"`
				Origin    string `json:"origin"`
				Workspace string `json:"workspace_id"`
			} `json:"job"`
			Steps []struct {
				Status  int    `json:"status"`
				Origin  string `json:"origin"`
				AssetID string `json:"output_asset_id"`
			} `json:"step_runs"`
		}
		status, body = doJSONBearer(t, addr, http.MethodGet, "/api/v1/jobs/"+url.PathEscape(m.JobID), token, nil)
		if status != http.StatusOK || json.Unmarshal(body, &job) != nil || job.Job.Status != 4 || job.Job.SourceID != "video:"+m.SourceID || job.Job.Origin != "legacy_import" || job.Job.Workspace != workspace || len(job.Steps) != 4 {
			t.Fatalf("member %s is not a completed historical Video Job", m.SourceID)
		}
		for _, step := range job.Steps {
			if step.Status != 5 || step.Origin != "legacy_import" || step.AssetID == "" {
				t.Fatal("historical member has an invalid completed step")
			}
		}
	}
	for _, object := range []media{item.Video, item.Thumbnail, item.Manual} {
		if object.ObjectPath == "" {
			if object.SHA256 != "" || object.URL != "" {
				t.Fatal("empty media field contains partial facts")
			}
			continue
		}
		if !strings.Contains(object.ObjectPath, "/historical_publication_import/") || len(object.SHA256) != 64 || object.URL == "" {
			t.Fatal("publication media facts are incomplete")
		}
		req, err := http.NewRequest(http.MethodGet, object.URL, nil)
		if err != nil {
			t.Fatal("invalid media URL")
		}
		resp, err := (&http.Client{Timeout: time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
		if err != nil {
			t.Fatal("publication media fetch failed")
		}
		hash := sha256.New()
		size, readErr := io.Copy(hash, io.LimitReader(resp.Body, (256<<20)+1))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || readErr != nil || size == 0 || size > 256<<20 || hex.EncodeToString(hash.Sum(nil)) != object.SHA256 {
			t.Fatal("target publication media checksum mismatch")
		}
	}
	status, _ = doJSONBearer(t, addr, http.MethodGet, path, "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated publication read HTTP %d", status)
	}
	status, _ = doJSONBearer(t, addr, http.MethodGet, "/api/v1/historical-video-pushes?workspace_id=nonexistent-test-workspace", token, nil)
	if status != http.StatusNotFound {
		t.Fatalf("foreign/missing Workspace read HTTP %d", status)
	}
	var listed struct {
		Items []push `json:"items"`
	}
	status, body = doJSONBearer(t, addr, http.MethodGet, "/api/v1/historical-video-pushes?workspace_id="+url.QueryEscape(workspace)+"&page_size=100", token, nil)
	if status != http.StatusOK || json.Unmarshal(body, &listed) != nil {
		t.Fatal("publication list failed")
	}
	found := false
	for _, entry := range listed.Items {
		if entry.Workspace != workspace {
			t.Fatal("list leaked foreign Workspace")
		}
		found = found || entry.ID == id
	}
	if !found {
		t.Fatal("sample missing from Workspace list")
	}
}
