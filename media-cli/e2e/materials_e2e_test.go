// Public CRUD regression uses the unified gateway and a provisioned API Token.
// Binding/owner-list assertions remain in materials_reachability_e2e_test.go;
// passing CRUD does not imply those deferred capabilities are implemented.
package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

func skipIfNoMaterialsServer(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("MEDIA_MATERIALS_ADDR")
	if addr == "" {
		t.Skip("configure MEDIA_MATERIALS_ADDR for private service tests")
	}
	return addr
}

func TestMaterialsE2E(t *testing.T) {
	f := newMaterialsAPIFixture(t)
	var kinds struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if json.Unmarshal(f.request(t, "GET", "/api/v1/asset-kinds?resource_type=MATERIAL&media_type=IMAGE", nil, 200), &kinds) != nil || kinds.Total == 0 {
		t.Fatal("image material kinds unavailable")
	}
	// Use a registered image kind; do not mutate the global kind registry.
	kind := kinds.Items[0].Name
	for _, resource := range []struct {
		name, path     string
		create, update map[string]any
	}{
		{"material", "/api/v1/materials", map[string]any{"name": "e2e-material", "asset_kind": kind, "content": "API Token fixture", "visibility": "private"}, map[string]any{"name": "updated-material", "content": "updated fixture"}},
		{"virtual-ip", "/api/v1/virtual-ips", map[string]any{"name": "e2e-vip", "visibility": "private", "persona_profile": "{}"}, map[string]any{"name": "updated-vip", "persona_profile": "{\"description\":\"API fixture\"}"}},
		{"product", "/api/v1/products", map[string]any{"name": "e2e-product", "target_audience": "test"}, map[string]any{"name": "updated-product", "target_audience": "updated test"}},
	} {
		t.Run(resource.name, func(t *testing.T) {
			var created struct {
				ID           string `json:"id"`
				Organization string `json:"organization_id"`
				Owner        string `json:"owner_user_id"`
				Creator      string `json:"created_by"`
			}
			// Caller identity must come from verified JWT, not supplied body/query.
			resource.create["owner_user_id"] = "spoofed-owner"
			resource.create["created_by"] = "spoofed-creator"
			body := f.request(t, "POST", resource.path+"?scope=ai_clothing", resource.create, http.StatusCreated)
			if json.Unmarshal(body, &created) != nil || created.ID == "" || created.Organization != f.organization || (resource.name == "product" && created.Creator != f.user) || (resource.name != "product" && created.Owner != f.user) {
				t.Fatal("created resource lost verified ownership")
			}
			path := resource.path + "/" + created.ID
			deleted := false
			t.Cleanup(func() {
				if !deleted {
					f.request(t, "DELETE", path, nil, 200)
				}
			})
			f.request(t, "GET", path, nil, 200)
			f.request(t, "GET", path+"?organization_id=not-a-member", nil, 404)
			f.request(t, "PUT", path, resource.update, 200)
			var updated struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(f.request(t, "GET", path, nil, 200), &updated) != nil || updated.Name != resource.update["name"] {
				t.Fatal("updated resource was not persisted")
			}
			var list struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
				Total int `json:"total"`
			}
			if json.Unmarshal(f.request(t, "GET", resource.path+"?scope=ai_clothing&page=1&page_size=1", nil, 200), &list) != nil || len(list.Items) != 1 || list.Total < 1 {
				t.Fatal("paged resource list unavailable")
			}
			f.request(t, "DELETE", path, nil, 200)
			deleted = true
			f.request(t, "GET", path, nil, 404)
		})
	}
}
