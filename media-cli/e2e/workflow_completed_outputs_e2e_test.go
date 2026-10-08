package e2e

import (
	"crypto/sha256"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Read an existing completed Job; never submit, resume, approve or start workers.
// Borrowed credentials stay at the gateway, not on the signed storage request.
func TestWorkflowCompletedOutputsGateway(t *testing.T) {
	addr, token, org := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_ORGANIZATION_ID")
	jobID := os.Getenv("MEDIA_E2E_COMPLETED_JOB_ID")
	if addr == "" || token == "" || org == "" || jobID == "" {
		t.Skip("configure gateway, User API Token, Organization and MEDIA_E2E_COMPLETED_JOB_ID")
	}
	read := func(path string, out any) {
		t.Helper()
		status, data := doJSONBearer(t, addr, http.MethodGet, path, token, nil)
		if status != http.StatusOK || json.Unmarshal(data, out) != nil {
			t.Fatal("gateway result unavailable or malformed")
		}
	}
	var user struct {
		ID string `json:"id"`
	}
	read("/api/auth/me", &user)
	var state struct {
		Job struct {
			ID     string `json:"id"`
			Owner  string `json:"submitted_by"`
			Status int    `json:"status"`
		} `json:"job"`
		Steps []struct {
			Key    string `json:"step_key"`
			Status int    `json:"status"`
			Asset  string `json:"output_asset_id"`
		} `json:"step_runs"`
	}
	read("/api/v1/jobs/"+url.PathEscape(jobID), &state)
	if user.ID == "" || state.Job.ID != jobID || state.Job.Owner != user.ID || state.Job.Status != 4 || len(state.Steps) != 4 {
		t.Fatal("expected an owned, completed four-node Job")
	}
	want := map[string]string{"model_image": "image/", "style_image": "image/", "first_frame": "image/", "video": "video/"}
	storage := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, step := range state.Steps {
		mediaPrefix, expected := want[step.Key]
		if !expected || step.Status != 5 || step.Asset == "" {
			t.Fatal("missing, duplicate or incomplete generation node")
		}
		delete(want, step.Key)
		var asset struct {
			ID   string `json:"id"`
			Path string `json:"object_path"`
			URL  string `json:"url"`
		}
		read("/api/v1/generated-assets/"+url.PathEscape(step.Asset)+"?organization_id="+url.QueryEscape(org), &asset)
		if asset.ID != step.Asset || !strings.HasPrefix(asset.Path, "materials/"+org+"/") || asset.URL == "" {
			t.Fatalf("node %s output is not backed by the target Organization storage", step.Key)
		}
		location, err := url.Parse(asset.URL)
		if err != nil || location.Host == "" || location.User != nil || (location.Scheme != "http" && location.Scheme != "https") {
			t.Fatal("invalid storage access URL")
		}
		req, err := http.NewRequest(http.MethodGet, asset.URL, nil)
		if err != nil {
			t.Fatal("cannot construct storage probe")
		}
		req.Header.Set("Range", "bytes=0-0")
		response, err := storage.Do(req)
		if err != nil {
			t.Fatalf("node %s storage probe failed", step.Key)
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 2))
		response.Body.Close()
		contentType, _, typeErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
		contentRange := strings.TrimPrefix(response.Header.Get("Content-Range"), "bytes 0-0/")
		size, sizeErr := strconv.ParseInt(contentRange, 10, 64)
		if response.StatusCode != http.StatusPartialContent || readErr != nil || len(data) != 1 || typeErr != nil || !strings.HasPrefix(contentType, mediaPrefix) || sizeErr != nil || size <= 0 {
			t.Fatalf("node %s output has invalid Range response or media type (status=%d)", step.Key, response.StatusCode)
		}
		t.Logf("node %s persisted output readable (Range 206, %s)", step.Key, contentType)
		if os.Getenv("MEDIA_E2E_PROVIDER_FILE_RESULTS") == "1" {
			if !strings.Contains(asset.Path, "/worker_generated/") {
				t.Fatal("file fixture reused a pre-uploaded legacy object")
			}
			position, ext := 0, ".png"
			if step.Key == "model_image" {
				position = 1
			}
			if step.Key == "video" {
				ext = ".mp4"
			}
			fixture := filepath.Join(filepath.Dir(os.Getenv("MEDIA_E2E_PROVIDER_FIXTURE_CONFIG")), step.Key+"-"+strconv.Itoa(position)+ext)
			file, err := os.Open(fixture)
			if err != nil {
				t.Fatal("private expected file unavailable")
			}
			wantDigest := sha256.New()
			wantSize, err := io.Copy(wantDigest, file)
			file.Close()
			if err != nil || wantSize != size {
				t.Fatal("fixture size differs from stored output")
			}
			full, err := storage.Get(asset.URL)
			if err != nil {
				t.Fatal("full object read unavailable")
			}
			gotDigest := sha256.New()
			gotSize, readErr := io.Copy(gotDigest, io.LimitReader(full.Body, size+1))
			full.Body.Close()
			if full.StatusCode != 200 || readErr != nil || gotSize != size || string(gotDigest.Sum(nil)) != string(wantDigest.Sum(nil)) {
				t.Fatal("uploaded result bytes differ from client fixture")
			}
			t.Logf("node %s uploaded bytes match local fixture (SHA256)", step.Key)
		}
	}
}

func TestWorkflowCompletedOutputsProbeContract(t *testing.T) {
	const credential = "test-only-credential"
	var apiReads, mediaReads atomic.Int32
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Range") != "bytes=0-0" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("storage probe leaked credentials or changed the read-only contract")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		contentType := "image/png"
		if r.URL.Path == "/video" {
			contentType = "video/mp4"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Range", "bytes 0-0/100")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{1})
		mediaReads.Add(1)
	}))
	t.Cleanup(media.Close)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer "+credential || r.Header.Get("Cookie") != "" {
			t.Error("gateway probe changed identity or attempted a mutation")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		apiReads.Add(1)
		switch {
		case r.URL.Path == "/api/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "owner"})
		case r.URL.Path == "/api/v1/jobs/job":
			steps := []map[string]any{}
			for _, key := range []string{"model_image", "style_image", "first_frame", "video"} {
				steps = append(steps, map[string]any{"step_key": key, "status": 5, "output_asset_id": key})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"job": map[string]any{"id": "job", "submitted_by": "owner", "status": 4}, "step_runs": steps})
		case strings.HasPrefix(r.URL.Path, "/api/v1/generated-assets/") && r.URL.Query().Get("organization_id") == "org":
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/generated-assets/")
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "object_path": "materials/org/scope/" + id, "url": media.URL + "/" + id})
		default:
			t.Error("unexpected gateway route or missing organization")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(gateway.Close)
	t.Setenv("MEDIA_GATEWAY_ADDR", gateway.URL)
	t.Setenv("MEDIA_API_TOKEN", credential)
	t.Setenv("MEDIA_E2E_ORGANIZATION_ID", "org")
	t.Setenv("MEDIA_E2E_COMPLETED_JOB_ID", "job")
	// This protocol fixture serves tiny pre-existing objects, not file uploads.
	// An opt-in deployment gate in the same shell must not change its contract.
	t.Setenv("MEDIA_E2E_PROVIDER_FILE_RESULTS", "")
	TestWorkflowCompletedOutputsGateway(t)
	if apiReads.Load() != 6 || mediaReads.Load() != 4 {
		t.Fatal("not all four node outputs were independently probed")
	}
}
