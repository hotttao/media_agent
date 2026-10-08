package e2e

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Own unique definitions only. No Job, worker, model call or old data edit.
func TestResourcePermissionsGatewayAndPublicationCLI(t *testing.T) {
	if os.Getenv("MEDIA_E2E_RESOURCE_PERMISSIONS") != "1" {
		t.Skip("explicit owned resource permissions gate")
	}
	f := newMaterialsAPIFixture(t)
	source, workspace, binary := os.Getenv("MEDIA_E2E_PUBLICATION_SOURCE_ID"), os.Getenv("MEDIA_E2E_RESOURCE_WORKSPACE_ID"), os.Getenv("MEDIA_E2E_CLI_BINARY")
	if source == "" || workspace == "" || !filepath.IsAbs(binary) {
		t.Fatal("owned source, Workspace and independent CLI required")
	}
	request := func(method, path string, body, out any) {
		t.Helper()
		status, data := doJSONBearer(t, f.gateway, method, path, f.token, body)
		if status != 200 && !(method == "POST" && status == 202) {
			t.Fatalf("resource operation %s status=%d; payload suppressed", path, status)
		}
		if out != nil && json.Unmarshal(data, out) != nil {
			t.Fatal("malformed resource response")
		}
	}
	var w struct {
		Scope  string           `json:"scope"`
		Output string           `json:"output_step_key"`
		Steps  []map[string]any `json:"steps"`
		Owner  string           `json:"created_by"`
	}
	request("GET", "/api/v1/workflows/"+url.PathEscape(source), nil, &w)
	if w.Owner != f.user || w.Scope == "" || len(w.Steps) == 0 {
		t.Fatal("refusing unrelated source")
	}
	var workspaceInfo struct {
		ID           string `json:"id"`
		Organization string `json:"organization_id"`
		Team         string `json:"team_id"`
	}
	request("GET", "/api/v1/workspaces/"+workspace, nil, &workspaceInfo)
	if workspaceInfo.ID != workspace || workspaceInfo.Organization != f.organization || workspaceInfo.Team == "" {
		t.Fatal("target Workspace mismatch")
	}
	prefix := "acl-gateway-" + time.Now().UTC().Format("20060102T150405.000000000")
	for i, step := range w.Steps {
		id, ok := step["template_id"].(string)
		if !ok || id == "" {
			t.Fatal("missing template ref")
		}
		version, ok := step["template_version"].(float64)
		if !ok || version <= 0 {
			t.Fatal("missing template version")
		}
		var template map[string]any
		request("GET", "/api/v1/templates/"+url.PathEscape(id)+"/versions/"+strconv.Itoa(int(version)), nil, &template)
		body := map[string]any{"organization_id": f.organization, "name": prefix + "-" + strconv.Itoa(i)}
		for _, field := range []string{"prompt_body", "inputs", "output_asset_kind", "platform", "gen_way", "gen_type", "needs_confirm", "candidate_count", "capability_key", "capability_version", "invocation_mapping"} {
			if value, ok := template[field]; ok {
				body[field] = value
			}
		}
		var created struct {
			ID string `json:"id"`
		}
		request("POST", "/api/v1/templates?scope="+url.QueryEscape(w.Scope), body, &created)
		if created.ID == "" {
			t.Fatal("no new Template id")
		}
		createdID := created.ID
		t.Cleanup(func() {
			status, _ := doJSONBearer(t, f.gateway, "POST", "/api/v1/templates/"+createdID+"/archive", f.token, map[string]any{})
			if status != 200 {
				t.Errorf("test Template archive status=%d", status)
			}
		})
		request("POST", "/api/v1/templates/"+createdID+"/publish", map[string]any{}, nil)
		request("POST", "/api/v1/templates/"+createdID+"/grants", map[string]any{"team_id": workspaceInfo.Team, "permission": "viewer", "granted_by": "spoofed-issuer"}, nil)
		request("POST", "/api/v1/templates/"+createdID+"/visibility", map[string]any{"visibility": "team"}, nil)
		request("DELETE", "/api/v1/templates/"+createdID+"/grants/"+workspaceInfo.Team, nil, nil)
		for _, token := range []string{""} {
			status, _ := doJSONBearer(t, f.gateway, "POST", "/api/v1/templates/"+createdID+"/publish", token, map[string]any{})
			if status != 401 && status != 403 {
				t.Fatal("anonymous publish not denied")
			}
		}
		step["template_id"], step["template_version"] = createdID, 1
	}
	status, _ := doJSONBearer(t, f.gateway, "POST", "/api/v1/templates?scope="+url.QueryEscape(w.Scope), f.token, map[string]any{"organization_id": "foreign", "name": prefix + "-forbidden"})
	if status != 403 {
		t.Fatalf("foreign enterprise create status=%d", status)
	}
	var created struct {
		ID string `json:"id"`
	}
	request("POST", "/api/v1/workflows?scope="+url.QueryEscape(w.Scope), map[string]any{"organization_id": f.organization, "workspace_id": workspace, "name": prefix, "output_step_key": w.Output, "steps": w.Steps}, &created)
	if created.ID == "" {
		t.Fatal("no new Workflow id")
	}
	t.Log("owned permission fixture workflow_id=" + created.ID)
	t.Cleanup(func() {
		status, _ := doJSONBearer(t, f.gateway, "POST", "/api/v1/workflows/"+created.ID+"/archive", f.token, map[string]any{})
		if status != 200 {
			t.Errorf("test Workflow archive status=%d", status)
		}
	})
	request("POST", "/api/v1/workflows/"+created.ID+"/grants", map[string]any{"team_id": workspaceInfo.Team, "permission": "editor", "granted_by": "spoofed-issuer"}, nil)
	request("POST", "/api/v1/workflows/"+created.ID+"/visibility", map[string]any{"visibility": "team"}, nil)
	callCLI := func(operation string) publicationReceipt {
		t.Helper()
		cmd := exec.Command(binary, "media_workflow", "workflow", operation, "--id", created.ID)
		cmd.Dir = ".."
		cmd.Env = append(os.Environ(), "APP_ENV=local", "MEDIA_WORKFLOW_ADDR="+f.gateway, "MEDIA_MATERIALS_ADDR="+f.gateway, "MEDIA_API_TOKEN="+f.token)
		output, err := cmd.Output()
		var receipt struct {
			ID       string `json:"id"`
			Delivery string `json:"publication_status"`
		}
		if err != nil || json.Unmarshal(output, &receipt) != nil || receipt.ID != created.ID {
			t.Fatal("independent resource CLI failed; output suppressed")
		}
		return publicationReceipt{ID: receipt.ID, PublicationStatus: receipt.Delivery}
	}
	callCLI("publish")
	for deadline := time.Now().Add(30 * time.Second); ; {
		receipt := callCLI("get")
		if receipt.PublicationStatus == "registered" {
			break
		}
		if receipt.PublicationStatus == "failed" || time.Now().After(deadline) {
			t.Fatal("publication did not converge; inspect original definition")
		}
		time.Sleep(100 * time.Millisecond)
	}
	request("DELETE", "/api/v1/workflows/"+created.ID+"/grants/"+workspaceInfo.Team, nil, nil)
}
