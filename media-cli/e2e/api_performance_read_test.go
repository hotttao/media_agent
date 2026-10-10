package e2e

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
)

// Read-only pagination/tenant contract gate. No business records or tokens are
// created; latency measurement is separate from deterministic correctness.
func TestAPIPerformanceReadGateway(t *testing.T) {
	addr, token, org := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_ORGANIZATION_ID")
	if addr == "" || token == "" || org == "" {
		t.Skip("existing gateway User Token and organization required")
	}
	type item struct {
		ID           string `json:"id"`
		Organization string `json:"organization_id"`
		URL          string `json:"url"`
		ObjectPath   string `json:"object_path"`
		Images       []struct {
			ID   string `json:"id"`
			Path string `json:"object_path"`
			Sort int    `json:"sort_order"`
		} `json:"images"`
	}
	type page struct {
		Items []item `json:"items"`
		Total int    `json:"total"`
	}
	for _, resource := range []string{"products", "generated-assets", "virtual-ips", "materials"} {
		t.Run(resource, func(t *testing.T) {
			read := func(size int, tenant, credential string) (int, page) {
				status, data := doJSONBearer(t, addr, "GET", fmt.Sprintf("/api/v1/%s?scope=ai_clothing&organization_id=%s&page_size=%d", resource, url.QueryEscape(tenant), size), credential, nil)
				var result page
				if status == 200 && json.Unmarshal(data, &result) != nil {
					t.Fatal("malformed page; response suppressed")
				}
				return status, result
			}
			status, small := read(20, org, token)
			largeStatus, large := read(200, org, token)
			if status != 200 || largeStatus != 200 || small.Total != large.Total || len(small.Items) > 20 || len(large.Items) > 200 || len(large.Items) < len(small.Items) {
				t.Fatal("pagination contract changed")
			}
			for i, row := range small.Items {
				if large.Items[i].ID != row.ID {
					t.Fatal("page prefix order changed")
				}
			}
			for _, row := range large.Items {
				if row.ID == "" || (resource != "generated-assets" && row.Organization != org) || (row.Organization != "" && row.Organization != org) {
					t.Fatal("missing identity or foreign organization")
				}
				// The public GeneratedAsset DTO has no organization_id field;
				// check the storage namespace and the denied tenant request instead.
				if resource == "generated-assets" && row.ObjectPath != "" && (row.URL == "" || !strings.HasPrefix(row.ObjectPath, "materials/"+org+"/")) {
					t.Fatal("stored asset has no access URL or belongs to a foreign namespace")
				}
				for i, image := range row.Images {
					if image.ID == "" || !strings.HasPrefix(image.Path, "materials/"+org+"/") || (i > 0 && row.Images[i-1].Sort > image.Sort) {
						t.Fatal("product image association/order changed")
					}
				}
			}
			if denied, result := read(20, "perf-no-membership-organization", token); denied == 200 {
				// GeneratedAsset does not accept an organization_id filter in
				// its IDL; scope is derived from the authenticated principal.
				if resource != "generated-assets" {
					t.Fatal("foreign organization accepted")
				}
				for _, row := range result.Items {
					if row.ObjectPath != "" && !strings.HasPrefix(row.ObjectPath, "materials/"+org+"/") {
						t.Fatal("principal-derived asset scope leaked a foreign object")
					}
				}
			}
			if denied, _ := read(20, org, ""); denied == 200 {
				t.Fatal("unauthenticated page accepted")
			}
		})
	}
}
