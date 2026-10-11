package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// Real API Token -> gateway -> Workflow publication -> Conductor definition.
// Only a unique owned definition is created and archived; no Job/model starts.
func TestWorkflowReusePublicationGateway(t *testing.T) {
	address, token := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN")
	source, workspace, organization := os.Getenv("MEDIA_E2E_PUBLICATION_SOURCE_ID"), os.Getenv("MEDIA_E2E_WORKSPACE_ID"), os.Getenv("MEDIA_E2E_ORGANIZATION_ID")
	engine := strings.TrimRight(os.Getenv("MEDIA_E2E_CONDUCTOR_URL"), "/")
	if address == "" || token == "" || source == "" || workspace == "" || organization == "" || engine == "" {
		t.Skip("explicit publication fixture configuration required")
	}
	query := "?organization_id=" + url.QueryEscape(organization)
	status, data := doJSONBearer(t, address, "GET", "/api/v1/workflows/"+source, token, nil)
	var definition struct {
		Scope  string           `json:"scope"`
		Output string           `json:"output_step_key"`
		Steps  []map[string]any `json:"steps"`
	}
	if status != 200 || json.Unmarshal(data, &definition) != nil || len(definition.Steps) == 0 {
		t.Fatalf("authorized source unavailable: HTTP %d", status)
	}
	for _, step := range definition.Steps {
		step["reuse_policy"] = "reuse_if_available"
		status, raw := doJSONBearer(t, address, "GET", "/api/v1/templates/"+step["template_id"].(string)+"/versions/"+fmt.Sprint(step["template_version"]), token, nil)
		var snapshot map[string]any
		if status != 200 || json.Unmarshal(raw, &snapshot) != nil {
			t.Fatal("authorized template snapshot unavailable")
		}
		capabilityKey, _ := snapshot["capability_key"].(string)
		status, raw = doJSONBearer(t, address, "GET", "/api/v1/capabilities/"+capabilityKey+"/versions/"+fmt.Sprint(snapshot["capability_version"]), token, nil)
		var contract map[string]any
		if status != 200 || json.Unmarshal(raw, &contract) != nil {
			t.Fatal("published capability contract unavailable")
		}
		snapshot["name"], snapshot["organization_id"] = "reuse-publication-template-"+time.Now().UTC().Format("20060102T150405.000000000"), organization
		snapshot["platform"], snapshot["gen_way"], snapshot["gen_type"] = contract["provider"], contract["generation_method"], contract["generation_type"]
		status, raw = doJSONBearer(t, address, "POST", "/api/v1/templates?scope="+url.QueryEscape(definition.Scope), token, snapshot)
		var template struct {
			ID string `json:"id"`
		}
		if status != 200 || json.Unmarshal(raw, &template) != nil || template.ID == "" {
			t.Fatalf("isolated template creation failed: HTTP %d", status)
		}
		t.Cleanup(func() {
			status, _ := doJSONBearer(t, address, "POST", "/api/v1/templates/"+template.ID+"/archive", token, map[string]any{})
			if status != 200 {
				t.Error("isolated template archive failed")
			}
		})
		status, _ = doJSONBearer(t, address, "POST", "/api/v1/templates/"+template.ID+"/publish", token, map[string]any{})
		if status != 200 {
			t.Fatalf("isolated template publication failed: HTTP %d", status)
		}
		step["template_id"], step["template_version"] = template.ID, 1
	}
	status, data = doJSONBearer(t, address, "POST", "/api/v1/workflows"+query+"&scope="+url.QueryEscape(definition.Scope), token, map[string]any{"organization_id": organization, "workspace_id": workspace, "name": "reuse-publication-gate-" + time.Now().UTC().Format("20060102T150405.000000000"), "output_step_key": definition.Output, "steps": definition.Steps})
	var created struct {
		ID string `json:"id"`
	}
	if status != 200 || json.Unmarshal(data, &created) != nil || created.ID == "" {
		t.Fatalf("isolated definition creation failed: HTTP %d", status)
	}
	t.Cleanup(func() {
		status, _ := doJSONBearer(t, address, "POST", "/api/v1/workflows/"+created.ID+"/archive"+query, token, map[string]any{})
		if status != 200 {
			t.Error("isolated definition archive failed")
		}
	})
	status, data = doJSONBearer(t, address, "POST", "/api/v1/workflows/"+created.ID+"/publish"+query, token, map[string]any{})
	var receipt publicationReceipt
	if (status != 200 && status != 202) || json.Unmarshal(data, &receipt) != nil || receipt.PublicationID == "" {
		t.Fatalf("publication acceptance failed: HTTP %d", status)
	}
	for deadline := time.Now().Add(45 * time.Second); ; {
		status, data = doJSONBearer(t, address, "GET", "/api/v1/workflows/"+created.ID+query, token, nil)
		var current publicationReceipt
		if status != 200 || json.Unmarshal(data, &current) != nil || current.PublicationID != receipt.PublicationID {
			t.Fatal("publication receipt was not retained")
		}
		if current.Status == 2 && current.PublicationStatus == "registered" {
			break
		}
		if current.PublicationStatus == "failed" || time.Now().After(deadline) {
			t.Fatal("publication did not register; do not resubmit")
		}
		time.Sleep(100 * time.Millisecond)
	}
	name := "media_workflow_" + strings.ReplaceAll(created.ID, "-", "_")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(engine + "/api/metadata/workflow/" + name + "?version=1")
	if err != nil {
		t.Fatal("registered definition observation unavailable")
	}
	defer response.Body.Close()
	var compiled map[string]any
	if response.StatusCode != 200 || json.NewDecoder(response.Body).Decode(&compiled) != nil {
		t.Fatal("registered native definition unavailable")
	}
	checks, gates := 0, 0
	var inspect func(any)
	inspect = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if v["name"] == "media_check_result_reuse_v1" && v["type"] == "SIMPLE" {
				checks++
				input, _ := v["inputParameters"].(map[string]any)
				if input["policy"] != "reuse_if_available" || input["identity"] == nil || input["provider_input"] == nil {
					t.Error("reuse contract not frozen in native task")
				}
			}
			if name, _ := v["name"].(string); strings.HasPrefix(name, "reuse_gate_") && v["type"] == "SWITCH" {
				gates++
			}
			for _, nested := range v {
				inspect(nested)
			}
		case []any:
			for _, nested := range v {
				inspect(nested)
			}
		}
	}
	inspect(compiled["tasks"])
	if checks != len(definition.Steps) || gates != checks {
		t.Fatalf("published native reuse plan incomplete: checks=%d gates=%d steps=%d", checks, gates, len(definition.Steps))
	}
	t.Logf("fresh publication registered %d native reuse branches; no Job submitted", checks)
}
