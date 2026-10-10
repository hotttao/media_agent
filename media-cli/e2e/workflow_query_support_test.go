package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Read-only gateway and independent CLI gate. No Job, Worker, grant or identity is created.
func TestWorkflowUIQuerySupportGatewayAndCLI(t *testing.T) {
	address, token, id, binary := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_QUERY_WORKFLOW_ID"), os.Getenv("MEDIA_E2E_CLI_BINARY")
	if address == "" || token == "" || id == "" || !filepath.IsAbs(binary) {
		t.Skip("gateway, API Token, owned workflow fixture and independent CLI binary required")
	}
	request := func(path string, out any) {
		t.Helper()
		status, data := doJSONBearer(t, address, "GET", path, token, nil)
		if status != 200 || json.Unmarshal(data, out) != nil {
			t.Fatalf("query gate status=%d; payload suppressed", status)
		}
		for _, field := range []string{`"conductor_task_def"`, `"managed_pool_key"`, `"internal_jwt"`} {
			if strings.Contains(string(data), field) {
				t.Fatal("internal field exposed")
			}
		}
	}
	var catalog struct {
		Items []struct {
			Key     string `json:"key"`
			Version int    `json:"version"`
		}
		Total int `json:"total"`
	}
	request("/api/v1/capabilities?page=1&page_size=200", &catalog)
	if len(catalog.Items) == 0 || catalog.Total < len(catalog.Items) {
		t.Fatal("empty published registry")
	}
	capability := catalog.Items[0]
	var contract struct {
		Key     string `json:"key"`
		Version int    `json:"version"`
	}
	request(fmt.Sprintf("/api/v1/capabilities/%s/versions/%d", capability.Key, capability.Version), &contract)
	if contract.Key != capability.Key || contract.Version != capability.Version {
		t.Fatal("exact contract mismatch")
	}
	var workflow struct {
		ID         string   `json:"id"`
		Version    int      `json:"version"`
		Permission string   `json:"my_permission"`
		Actions    []string `json:"allowed_actions"`
		Steps      []struct {
			Template string `json:"template_id"`
		} `json:"steps"`
	}
	request("/api/v1/workflows/"+id, &workflow)
	if workflow.ID != id || workflow.Permission != "owner" || len(workflow.Steps) == 0 {
		t.Fatal("owned read fixture required")
	}
	var frozen struct {
		Version int      `json:"version"`
		Source  string   `json:"definition_source"`
		Actions []string `json:"allowed_actions"`
	}
	request(fmt.Sprintf("/api/v1/workflows/%s/versions/%d", id, workflow.Version), &frozen)
	if frozen.Version != workflow.Version || frozen.Source != "snapshot" || len(frozen.Actions) != 1 || frozen.Actions[0] != "read" {
		t.Fatal("frozen read projection invalid")
	}
	var grants struct {
		Items []map[string]any `json:"items"`
	}
	request("/api/v1/workflows/"+id+"/grants", &grants)
	template := workflow.Steps[0].Template
	var tpl struct {
		ID         string `json:"id"`
		Permission string `json:"my_permission"`
	}
	request("/api/v1/templates/"+template, &tpl)
	if tpl.Permission != "owner" {
		t.Fatal("fixture template must be owned")
	}
	request("/api/v1/templates/"+template+"/grants", &grants)
	for _, args := range [][]string{
		{"capability", "list", "--page-size", "200"}, {"capability", "get", "--key", capability.Key, "--version", fmt.Sprint(capability.Version)},
		{"template", "get", "--id", template}, {"template", "grants", "--id", template},
		{"workflow", "version", "--id", id, "--version", fmt.Sprint(workflow.Version)}, {"workflow", "grants", "--id", id},
	} {
		cmd := exec.Command(binary, append([]string{"media_workflow"}, args...)...)
		cmd.Dir = ".."
		cmd.Env = append(os.Environ(), "APP_ENV=local", "MEDIA_WORKFLOW_ADDR="+address, "MEDIA_MATERIALS_ADDR="+address, "MEDIA_API_TOKEN="+token)
		output, err := cmd.Output()
		var result map[string]any
		if err != nil || json.Unmarshal(output, &result) != nil || result == nil {
			t.Fatalf("independent CLI query %s %s failed; output suppressed", args[0], args[1])
		}
	}
	for path, want := range map[string]int{"/api/v1/capabilities?page_size=201": 400, "/api/v1/capabilities/not_a_capability/versions/1": 404, fmt.Sprintf("/api/v1/workflows/%s/versions/%d", id, workflow.Version+1): 404} {
		status, _ := doJSONBearer(t, address, "GET", path, token, nil)
		if status != want {
			t.Fatalf("negative query status=%d want=%d", status, want)
		}
	}
}
