package mcp_access

import "testing"

func TestMaterialsDelegationAllowsOnlyRequiredReads(t *testing.T) {
	for _, c := range []struct {
		operation, method, path string
		allowed                 bool
	}{
		{"jobs_submit", "GET", "/api/v1/products/p", true},
		{"jobs_submit", "GET", "/api/v1/virtual-ips/v", true},
		{"jobs_submit", "GET", "/api/v1/library-objects:resolve", true},
		{"jobs_submit", "POST", "/api/v1/products", false},
		{"jobs_submit", "GET", "/api/v1/generated-assets/a", false},
		{"jobs_get", "GET", "/api/v1/generated-assets/a", true},
		{"jobs_get", "GET", "/api/v1/generated-assets/by-source", false},
		{"jobs_get", "GET", "/api/v1/products/p", false},
		{"templates_get", "GET", "/api/v1/library-objects:resolve", false},
		{"jobs_get", "GET", "/api/v1/generated-assets/%2e%2e", false},
	} {
		if got := Allowed(c.operation, c.method, c.path); got != c.allowed {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}

func TestMaterialQueryRoutesAreReadOnlyAndOperationBound(t *testing.T) {
	for op, route := range map[string]string{"virtual_ips_list": "virtual-ips", "virtual_ips_get": "virtual-ips/item", "materials_list": "materials", "materials_get": "materials/item", "generated_assets_list": "generated-assets", "generated_assets_get": "generated-assets/item"} {
		if !Allowed(op, "GET", "/api/v1/"+route) || Allowed(op, "POST", "/api/v1/"+route) || Allowed(op, "GET", "/api/v1/products/item") || Allowed(op, "GET", "/api/v1/generated-assets/by-source") {
			t.Fatal("query route widened", op)
		}
	}
}
