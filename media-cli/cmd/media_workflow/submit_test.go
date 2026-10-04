package media_workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	configpb "media_agent/hertz_gen/config"
	jobpb "media_agent/hertz_gen/model/media_workflow/job"
)

func TestSubmitUsesPublicSelectionsAndResolvedMedia(t *testing.T) {
	var submits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer test-only-token" || r.Header.Get("Cookie") != "" {
			t.Error("public client lost API identity or sent cookies")
			w.WriteHeader(401)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && !strings.Contains(r.URL.Path, "workflows") && !strings.Contains(r.URL.Path, "jobs") && r.URL.Query().Get("organization_id") != "org" {
			w.WriteHeader(404)
			return
		}
		var result any
		switch r.URL.Path {
		case "/api/v1/workflows/wf":
			result = map[string]any{"id": "wf", "version": 1, "status": 2, "scope": "scope", "input_schema": map[string]any{"entries": []map[string]any{
				{"name": "product", "resource_type": "PRODUCT", "required": true},
				{"name": "virtual_ip", "resource_type": "VIRTUAL_IP", "required": true},
				{"name": "move", "resource_type": "MATERIAL", "asset_kind": "movement", "required": true},
				{"name": "reference", "resource_type": "ASSET", "asset_kind": "first_frame"},
			}}}
		case "/api/v1/products/p":
			result = map[string]any{"id": "p", "organization_id": "org", "scope": "scope", "images": []map[string]any{
				{"object_path": "materials/org/scope/other"}, {"object_path": "materials/org/scope/main", "is_main": true},
			}}
		case "/api/v1/virtual-ips/v":
			result = map[string]any{"id": "v", "organization_id": "org", "scope": "scope", "avatar_object_path": "materials/org/scope/avatar", "full_body_object_path": "materials/org/scope/body", "three_view_object_path": "materials/org/scope/three", "nine_view_object_path": "materials/org/scope/nine"}
		case "/api/v1/materials/m":
			result = map[string]any{"id": "m", "organization_id": "org", "scope": "scope", "asset_kind": "movement", "content": "walk"}
		case "/api/v1/generated-assets/a":
			result = map[string]any{"id": "a", "scope": "scope", "asset_kind": "first_frame", "object_path": "materials/org/scope/reference"}
		case "/api/v1/library-objects:resolve":
			result = map[string]any{"url": "https://storage.example/" + r.URL.Query().Get("object_path") + "?signature=test-only"}
		case "/api/v1/jobs":
			if r.Method != http.MethodPost || r.URL.Query().Get("submitted_by") != "" {
				t.Error("submission method or caller identity incorrect")
				w.WriteHeader(400)
				return
			}
			var body jobpb.SubmitJobReq
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.SubmittedBy != "" || body.Input == nil || body.WorkflowVersion != 1 {
				t.Error("submission lost bundle or forged identity")
				w.WriteHeader(400)
				return
			}
			submits.Add(1)
			result = map[string]any{"job_id": "job", "status": 1}
		default:
			t.Error("unexpected route: legacy Reachability must not be called")
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	t.Cleanup(server.Close)
	t.Setenv("MEDIA_API_TOKEN", "test-only-token")
	Init(&configpb.Config{Client: map[string]*configpb.AppClientConfig{
		"media_materials": {BaseDomain: server.URL}, "media_workflow": {BaseDomain: server.URL},
	}})
	selection := submitSelection{organization: "org", workflow: "wf", product: "p", virtualIP: "v", materials: map[string]string{"move": "m"}, assets: map[string]string{"reference": "a"}}
	request, err := buildSubmitRequest(context.Background(), selection)
	if err != nil {
		t.Fatal(err)
	}
	input := request.Input
	if !strings.Contains(input.Product.ImageUrl, "/main?") || !strings.Contains(input.VirtualIp.FullBodyUrl, "/body?") || !strings.Contains(input.VirtualIp.ThreeViewUrl, "/three?") || !strings.Contains(input.VirtualIp.NineViewUrl, "/nine?") || input.Material[0].Url != "" || input.Material[0].Content != "walk" || !strings.Contains(input.Asset[0].Url, "/reference?") || request.SubmittedBy != "" {
		t.Fatal("selected bundle did not preserve main/full-body/view/reference images and text-only movement")
	}
	for _, change := range []func(*submitSelection){
		func(s *submitSelection) { s.materials = nil },
		func(s *submitSelection) { s.materials = map[string]string{"unknown": "m"} },
		func(s *submitSelection) { s.organization = "foreign" },
		func(s *submitSelection) { s.version = 2 },
	} {
		invalid := selection
		change(&invalid)
		if _, err := buildSubmitRequest(context.Background(), invalid); err == nil {
			t.Fatal("invalid or inaccessible selection was accepted")
		}
	}
	args := []string{"--organization-id", "org", "--workflow-id", "wf", "--product-id", "p", "--virtual-ip-id", "v", "--material", "move=m", "--asset", "reference=a"}
	for _, dry := range []bool{true, false} {
		c := newSubmitJobCommand()
		var output bytes.Buffer
		c.SetOut(&output)
		c.SetArgs(append(append([]string{}, args...), "--dry-run="+map[bool]string{true: "true", false: "false"}[dry]))
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(output.String(), "signature=") || strings.Contains(output.String(), "test-only-token") {
			t.Fatal("command leaked credential or signed URL")
		}
		if dry && submits.Load() != 0 {
			t.Fatal("dry run submitted a Job")
		}
	}
	if submits.Load() != 1 {
		t.Fatal("normal invocation must submit exactly once")
	}
}
