package e2e

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Separate CLI processes must carry the same intent key across submissions.
// This cancellation-only gate never starts a fixture Worker or model call.
func TestWorkflowSubmitIdempotencyCLI(t *testing.T) {
	if os.Getenv("MEDIA_E2E_SUBMIT_CLI_SAMPLE") != "1" {
		t.Skip("explicit CLI submission sample")
	}
	binary, address, token, org, workspace, wf := os.Getenv("MEDIA_E2E_CLI_BINARY"), os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_ORGANIZATION_ID"), os.Getenv("MEDIA_E2E_WORKSPACE_ID"), os.Getenv("MEDIA_E2E_SAMPLE_WORKFLOW_ID")
	if !filepath.IsAbs(binary) || address == "" || token == "" || org == "" || workspace == "" || wf == "" {
		t.Fatal("CLI binary and authorized fixture scope required")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal("test intent identity unavailable")
	}
	args := []string{"media_workflow", "submit-job", "--organization-id", org, "--workspace-id", workspace, "--workflow-id", wf, "--product-id", envOr("MEDIA_E2E_SAMPLE_PRODUCT_ID", "prd_9af6be4e-1b78-4c52-85ec-35fa7234bd50"), "--virtual-ip-id", envOr("MEDIA_E2E_SAMPLE_VIRTUAL_IP_ID", "vip_e3393dcc-faf6-423f-bced-692ac22d0988"), "--idempotency-key", "cli-submit:" + hex.EncodeToString(nonce[:])}
	for _, kind := range []string{"movement", "pose", "scene"} {
		status, data := doJSONBearer(t, address, "GET", "/api/v1/materials?organization_id="+org+"&scope=ai_clothing&asset_kind="+kind+"&page_size=1", token, nil)
		var page struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		if status != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 1 || page.Items[0].ID == "" {
			t.Fatal("required fixture material unavailable")
		}
		args = append(args, "--material", kind+"="+page.Items[0].ID)
	}
	run := func() string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Dir = ".."
		cmd.Env = append(os.Environ(), "APP_ENV=local", "MEDIA_WORKFLOW_ADDR="+address, "MEDIA_MATERIALS_ADDR="+address)
		output, err := cmd.Output()
		if err != nil {
			t.Fatal("independent Submit command failed; query before retrying; private output suppressed")
		}
		for _, word := range strings.Fields(string(output)) {
			if strings.HasPrefix(word, "job_id=job_") {
				return strings.TrimPrefix(word, "job_id=")
			}
		}
		t.Fatal("CLI returned no acknowledged Job")
		return ""
	}
	id := run()
	t.Log("single CLI submission sample job_id=" + id)
	t.Cleanup(func() {
		status, _ := doJSONBearer(t, address, "POST", "/api/v1/jobs/"+id+"/cancel", token, map[string]any{})
		if status != 200 {
			t.Error("sample cleanup failed")
		}
	})
	if replay := run(); replay != id {
		t.Fatal("independent process created duplicate Job")
	}
	status, _ := doJSONBearer(t, address, "POST", "/api/v1/jobs/"+id+"/cancel", token, map[string]any{})
	if status != 200 {
		t.Fatal("sample cancellation failed")
	}
	if replay := run(); replay != id {
		t.Fatal("terminal CLI replay created duplicate Job")
	}
}
