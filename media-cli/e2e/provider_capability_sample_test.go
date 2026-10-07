package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Prepare a private configuration for an already acknowledged CLI Job. This
// test never submits another Job or starts a Worker/service. The operator
// launches the real external client after this gate, then runs execution.
func TestProviderCapabilityFixtureConfig(t *testing.T) {
	if os.Getenv("MEDIA_E2E_PROVIDER_SAMPLE") != "1" {
		t.Skip("explicit single Provider sample")
	}
	address, token, org, jobID := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_ORGANIZATION_ID"), os.Getenv("MEDIA_E2E_SAMPLE_JOB_ID")
	configPath := os.Getenv("MEDIA_E2E_PROVIDER_FIXTURE_CONFIG")
	if address == "" || token == "" || org == "" || jobID == "" || !filepath.IsAbs(configPath) {
		t.Fatal("existing sample identity and absolute private config path required")
	}
	read := func(path string, out any) {
		t.Helper()
		status, data := doJSONBearer(t, address, "GET", path, token, nil)
		if status != 200 || json.Unmarshal(data, out) != nil {
			t.Fatal("Provider sample preflight read failed")
		}
	}
	var me struct {
		ID string `json:"id"`
	}
	read("/api/auth/me", &me)
	var state struct {
		Job struct {
			Owner        string `json:"submitted_by"`
			Organization string `json:"organization_id"`
			Workflow     string `json:"workflow_id"`
			Version      int    `json:"workflow_version"`
			Status       int    `json:"status"`
		} `json:"job"`
	}
	read("/api/v1/jobs/"+jobID, &state)
	if me.ID == "" || state.Job.Owner != me.ID || state.Job.Organization != org || (state.Job.Status != 1 && state.Job.Status != 2) {
		t.Fatal("refusing to prepare unrelated or terminal sample")
	}
	var workflow struct {
		Version int `json:"version"`
		Steps   []struct {
			Key      string `json:"key"`
			Template string `json:"template_id"`
			Version  int    `json:"template_version"`
		} `json:"steps"`
	}
	read("/api/v1/workflows/"+state.Job.Workflow, &workflow)
	if workflow.Version != state.Job.Version || len(workflow.Steps) != 4 {
		t.Fatal("sample must reference unchanged four-node publication")
	}
	capabilities := map[string]string{}
	for _, step := range workflow.Steps {
		var tpl struct {
			Version           int                        `json:"version"`
			Key               string                     `json:"capability_key"`
			CapabilityVersion int                        `json:"capability_version"`
			Mapping           map[string]json.RawMessage `json:"invocation_mapping"`
		}
		read("/api/v1/templates/"+step.Template, &tpl)
		want := "jimeng.i2i.multi_image_prompt"
		if step.Key == "video" {
			want = "doubao.i2v.first_frame_prompt"
		}
		if tpl.Version != step.Version || tpl.Key != want || tpl.CapabilityVersion != 1 || tpl.Mapping["images"] == nil || tpl.Mapping["resources"] != nil {
			t.Fatal("sample still uses business Capability or incompatible Provider mapping")
		}
		capabilities[step.Key] = tpl.Key
	}
	var jobs struct {
		Total int `json:"total"`
		Items []struct {
			ID     string `json:"id"`
			Status int    `json:"status"`
		} `json:"items"`
	}
	for page := 1; ; page++ {
		read(fmt.Sprintf("/api/v1/jobs?page_size=200&page=%d", page), &jobs)
		for _, j := range jobs.Items {
			if j.Status == 1 || j.Status == 2 || j.Status == 3 {
				if j.ID != jobID {
					t.Fatal("another unfinished Job exists; do not launch fixture")
				}
			}
		}
		if page*200 >= jobs.Total {
			break
		}
		if page >= 1000 || len(jobs.Items) == 0 {
			t.Fatal("queue audit incomplete")
		}
	}
	type binding struct {
		Job        string   `json:"job_id"`
		Step       string   `json:"step_key"`
		Capability string   `json:"capability_key"`
		Version    int      `json:"capability_version"`
		URLs       []string `json:"result_urls,omitempty"`
		Files      []string `json:"result_files,omitempty"`
	}
	config := struct {
		Tasks []binding `json:"tasks"`
	}{Tasks: []binding{}}
	fixtures := map[string][]string{
		"model_image": {envOr("MEDIA_E2E_MODEL_RESULT_ASSET_ID", "gen_823f8029-8008-464a-b905-e37649b068ff"), envOr("MEDIA_E2E_MODEL_SECOND_RESULT_ASSET_ID", "gen_b1c91599-bee7-4c59-9ca6-d57ff5cbd9b5")},
		"style_image": {envOr("MEDIA_E2E_STYLE_RESULT_ASSET_ID", "gen_b1c91599-bee7-4c59-9ca6-d57ff5cbd9b5")},
		"first_frame": {envOr("MEDIA_E2E_FRAME_RESULT_ASSET_ID", "gen_b08582ca-2ac6-444f-bf22-f1fddc1afa8b")},
		"video":       {envOr("MEDIA_E2E_VIDEO_RESULT_ASSET_ID", "gen_5db3b9bd-8450-47d9-a028-93c76398ffbc")},
	}
	for _, step := range []string{"model_image", "style_image", "first_frame", "video"} {
		b := binding{Job: jobID, Step: step, Capability: capabilities[step], Version: 1}
		if b.Capability == "" {
			t.Fatal("missing sample step")
		}
		for position, assetID := range fixtures[step] {
			var asset struct {
				Path string `json:"object_path"`
			}
			read("/api/v1/generated-assets/"+url.PathEscape(assetID)+"?organization_id="+url.QueryEscape(org), &asset)
			if !strings.HasPrefix(asset.Path, "materials/"+org+"/") {
				t.Fatal("fixture object outside Organization")
			}
			var object struct {
				URL     string `json:"url"`
				Type    string `json:"content_type"`
				Size    int64  `json:"size_bytes"`
				Expires int64  `json:"expires_at"`
			}
			read("/api/v1/library-objects:resolve?organization_id="+url.QueryEscape(org)+"&object_path="+url.QueryEscape(asset.Path), &object)
			prefix := "image/"
			if step == "video" {
				prefix = "video/"
			}
			if object.URL == "" || !strings.HasPrefix(object.Type, prefix) || object.Size <= 0 || object.Expires < time.Now().Add(2*time.Minute).Unix() {
				t.Fatal("new Materials protocol or fixture media unavailable")
			}
			if os.Getenv("MEDIA_E2E_PROVIDER_FILE_RESULTS") == "1" {
				// Download known media without API Token. These private runtime
				// files are uploaded by the independent client, not this test.
				ext, limit := ".png", int64(16<<20)
				if step == "video" {
					ext, limit = ".mp4", 256<<20
				}
				if object.Size > limit {
					t.Fatal("fixture exceeds upload boundary")
				}
				path := filepath.Join(filepath.Dir(configPath), fmt.Sprintf("%s-%d%s", step, position, ext))
				file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
				if err != nil {
					t.Fatal("private result file creation refused")
				}
				download := &http.Client{Timeout: time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
				response, err := download.Get(object.URL)
				if err != nil {
					file.Close()
					t.Fatal("fixture download unavailable")
				}
				copied, copyErr := io.Copy(file, io.LimitReader(response.Body, object.Size+1))
				response.Body.Close()
				closeErr := file.Close()
				if response.StatusCode != 200 || copyErr != nil || closeErr != nil || copied != object.Size {
					t.Fatal("fixture download incomplete")
				}
				b.Files = append(b.Files, path)
			} else {
				b.URLs = append(b.URLs, object.URL)
			}
		}
		config.Tasks = append(config.Tasks, b)
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal("fixture encoding failed")
	}
	file, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("private config creation refused; do not overwrite existing fixture")
	}
	_, writeErr := file.Write(encoded)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("private fixture write failed")
	}
	t.Log("Provider contracts checked; private four-step fixture ready for existing job_id=" + jobID)
}

// Observe the same Job through HTTP while executing retry/confirm using real,
// separate CLI processes. No second Submit and no in-process fake Worker.
func TestProviderCapabilityExecution(t *testing.T) {
	if os.Getenv("MEDIA_E2E_PROVIDER_SAMPLE") != "1" {
		t.Skip("explicit single Provider sample")
	}
	binary, address, token, jobID := os.Getenv("MEDIA_E2E_CLI_BINARY"), os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_SAMPLE_JOB_ID")
	if !filepath.IsAbs(binary) || address == "" || token == "" || jobID == "" {
		t.Fatal("CLI binary and existing sample required")
	}
	cli := func(args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, append([]string{"media_workflow"}, args...)...)
		cmd.Dir = ".."
		cmd.Env = append(os.Environ(), "APP_ENV=local", "MEDIA_WORKFLOW_ADDR="+address, "MEDIA_MATERIALS_ADDR="+address)
		data, err := cmd.Output()
		if err != nil {
			t.Fatal("independent Provider sample CLI failed; private output suppressed; query before retrying")
		}
		return data
	}
	request := func(method, path string, body any, out any) {
		t.Helper()
		if method == "POST" && strings.HasPrefix(path, "/api/v1/step-runs/") && (strings.HasSuffix(path, "/retry") || strings.HasSuffix(path, "/confirm")) {
			parts := strings.Split(path, "/")
			id := parts[len(parts)-2]
			args := []string{"step-run", parts[len(parts)-1], "--id", id}
			values := body.(map[string]any)
			if strings.HasSuffix(path, "/retry") {
				args = append(args, "--reason", values["reason"].(string))
			} else {
				args = append(args, "--selected-index", fmt.Sprint(values["selected_index"]), "--idempotency-key", values["idempotency_key"].(string))
			}
			data := cli(args...)
			if out != nil && json.Unmarshal(data, out) != nil {
				t.Fatal("CLI receipt decode failed")
			}
			return
		}
		status, data := doJSONBearer(t, address, method, path, token, body)
		if status < 200 || status >= 300 {
			t.Fatalf("Provider sample %s request status=%d", method, status)
		}
		if out != nil && json.Unmarshal(data, out) != nil {
			t.Fatal("Provider sample response decode failed")
		}
	}
	t.Setenv("MEDIA_E2E_REGENERATE_STEP_KEY", "model_image")
	t.Setenv("MEDIA_E2E_RETRY_STEP_KEY", "model_image")
	t.Setenv("MEDIA_E2E_MULTIPLE_STEP_KEY", "model_image")
	t.Setenv("MEDIA_E2E_SELECT_CANDIDATE", "1")
	t.Setenv("MEDIA_E2E_REGENERATE_FEEDBACK", "保留服装纹理，调整构图")
	continuing := os.Getenv("MEDIA_E2E_PROVIDER_CONTINUE_AFTER_RETRY") == "1"
	if continuing {
		// Recovery observation must never issue another regenerate/retry.
		// Only use after the first gate acknowledged both commands for this Job.
		t.Setenv("MEDIA_E2E_REGENERATE_STEP_KEY", "")
		t.Setenv("MEDIA_E2E_RETRY_STEP_KEY", "")
		t.Setenv("MEDIA_E2E_REGENERATE_FEEDBACK", "")
	}
	awaitPersonalWorkerFixture(t, request, jobID)
	var summary struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Steps  []struct {
			Key     string `json:"step_key"`
			Retries int    `json:"retry_count"`
		} `json:"steps"`
	}
	if json.Unmarshal(cli("job", "get", "--id", jobID), &summary) != nil || summary.ID != jobID || summary.Status != "SUCCEEDED" {
		t.Fatal("independent CLI final query did not confirm success")
	}
	if continuing {
		found := false
		for _, step := range summary.Steps {
			if step.Key == "model_image" {
				found = step.Retries == 1
			}
		}
		if !found {
			t.Fatal("continued sample must retain exactly the previously acknowledged retry")
		}
	}
	t.Setenv("MEDIA_E2E_COMPLETED_JOB_ID", jobID)
	t.Run("stored_outputs", TestWorkflowCompletedOutputsGateway)
}
