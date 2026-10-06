package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Separate-process CLI gate. The external fixture Worker must explicitly fail
// the first model_image dispatch. No worker is implemented or started here.
func TestWorkflowRetryCLI(t *testing.T) {
	if os.Getenv("MEDIA_E2E_RETRY_CLI_SAMPLE") != "1" {
		t.Skip("explicit single-sample CLI retry gate")
	}
	binary, address, token, org, workspace, wf := os.Getenv("MEDIA_E2E_CLI_BINARY"), os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_ORGANIZATION_ID"), os.Getenv("MEDIA_E2E_WORKSPACE_ID"), os.Getenv("MEDIA_E2E_SAMPLE_WORKFLOW_ID")
	if !filepath.IsAbs(binary) || address == "" || token == "" || org == "" || workspace == "" || wf == "" {
		t.Fatal("CLI binary, gateway, token, organization, Workspace and workflow required")
	}
	get := func(path string, out any) {
		t.Helper()
		status, data := doJSONBearer(t, address, "GET", path, token, nil)
		if status != 200 || json.Unmarshal(data, out) != nil {
			t.Fatal("CLI sample preflight query failed")
		}
	}
	var page struct {
		Total int `json:"total"`
		Items []struct {
			Status int `json:"status"`
		} `json:"items"`
	}
	for p := 1; ; p++ {
		get(fmt.Sprintf("/api/v1/jobs?page_size=200&page=%d", p), &page)
		for _, j := range page.Items {
			if j.Status == 1 || j.Status == 2 || j.Status == 3 {
				t.Fatal("unfinished jobs exist; audit before starting CLI sample")
			}
		}
		if p*200 >= page.Total {
			break
		}
		if len(page.Items) == 0 || p >= 1000 {
			t.Fatal("full queue audit incomplete")
		}
	}
	args := []string{"submit-job", "--organization-id", org, "--workspace-id", workspace, "--workflow-id", wf, "--product-id", envOr("MEDIA_E2E_SAMPLE_PRODUCT_ID", "prd_9af6be4e-1b78-4c52-85ec-35fa7234bd50"), "--virtual-ip-id", envOr("MEDIA_E2E_SAMPLE_VIRTUAL_IP_ID", "vip_e3393dcc-faf6-423f-bced-692ac22d0988")}
	for _, kind := range []string{"movement", "pose", "scene"} {
		var materials struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		get("/api/v1/materials?organization_id="+org+"&scope=ai_clothing&asset_kind="+kind+"&page_size=1", &materials)
		if len(materials.Items) != 1 || materials.Items[0].ID == "" {
			t.Fatal("required fixture material missing")
		}
		args = append(args, "--material", kind+"="+materials.Items[0].ID)
	}
	cli := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command(binary, append([]string{"media_workflow"}, args...)...)
		cmd.Dir = ".." // CLI configuration is relative to the media-cli root.
		cmd.Env = append(os.Environ(), "APP_ENV=local", "MEDIA_WORKFLOW_ADDR="+address, "MEDIA_MATERIALS_ADDR="+address)
		output, err := cmd.Output()
		if err != nil {
			t.Fatal("independent workflow CLI command failed; output suppressed")
		}
		return output
	}
	cli(append(args, "--dry-run")...)
	output := string(cli(args...)) // Exactly one Submit. Never retry it on error.
	jobID := ""
	for _, word := range strings.Fields(output) {
		if strings.HasPrefix(word, "job_id=") {
			jobID = strings.TrimPrefix(word, "job_id=")
		}
	}
	if !strings.HasPrefix(jobID, "job_") {
		t.Fatal("CLI Submit did not acknowledge a Job")
	}
	t.Logf("single CLI retry sample job_id=%s", jobID)
	type step struct {
		ID      string `json:"id"`
		Key     string `json:"step_key"`
		Status  string `json:"status"`
		Count   int    `json:"candidate_count"`
		Retries int    `json:"retry_count"`
		Asset   string `json:"output_asset_id"`
	}
	deadline := time.Now().Add(2 * time.Minute)
	retried := false
	for time.Now().Before(deadline) {
		var state struct {
			Status string `json:"status"`
			Steps  []step `json:"steps"`
		}
		if json.Unmarshal(cli("job", "get", "--id", jobID), &state) != nil {
			t.Fatal("CLI Job summary invalid")
		}
		if state.Status == "FAILED" {
			if retried {
				t.Fatal("CLI sample failed again; preserve Job for investigation")
			}
			found := false
			for _, s := range state.Steps {
				if s.Key == "model_image" && s.Status == "SR_FAILED" {
					cli("step-run", "retry", "--id", s.ID, "--reason", "independent CLI retry gate")
					found = true
				}
			}
			if !found {
				t.Fatal("failed CLI sample lacks failed model StepRun")
			}
			retried = true
		} else if state.Status == "CANCELLED" {
			t.Fatal("CLI sample cancelled")
		}
		for _, s := range state.Steps {
			if s.Status == "SR_NEED_CONFIRM" {
				index := "0"
				if s.Key == "model_image" {
					if s.Count != 2 {
						t.Fatal("model CLI sample requires two distinct mock candidates")
					}
					index = "1"
				}
				cli("step-run", "confirm", "--id", s.ID, "--selected-index", index, "--idempotency-key", "cli-retry:"+jobID+":"+s.Key+":accept")
			}
		}
		if state.Status == "SUCCEEDED" {
			if !retried || len(state.Steps) != 4 {
				t.Fatal("CLI sample did not retry and finish four steps")
			}
			for _, s := range state.Steps {
				if s.Status != "SR_SUCCEEDED" || s.Asset == "" || (s.Key == "model_image" && s.Retries != 1) {
					t.Fatal("CLI sample output or retry count incomplete")
				}
			}
			t.Log("independent CLI Submit, query, failed Step Retry and candidate confirmation completed all four nodes")
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatal("CLI sample did not converge; do not resubmit")
}
