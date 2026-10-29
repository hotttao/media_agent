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
