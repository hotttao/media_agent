package mcp_access

import "testing"

func TestSubmissionDelegationCannotPublishOrComplete(t *testing.T) {
	for _, c := range []struct {
		operation, method, path string
		allowed                 bool
	}{
		{"jobs_submit", "POST", "/api/v1/jobs", true},
		{"jobs_submit", "GET", "/api/v1/workflows/w", true},
		{"jobs_submit", "GET", "/api/v1/templates/t/versions/1", true},
		{"jobs_submit", "POST", "/api/v1/workflows/w/publish", false},
		{"jobs_submit", "POST", "/api/worker/v1/tasks/t/complete", false},
		{"jobs_get", "POST", "/api/v1/jobs", false},
		{"templates_list", "GET", "/api/v1/jobs", false},
		{"jobs_submit", "POST", "/api/v1/jobs/../workflows", false},
		{"jobs_submit", "POST", "/api/v1/%6aobs", false},
		{"workflows_publish", "POST", "/api/v1/workflows/w/publish", true},
	} {
		if got := Allowed(c.operation, c.method, c.path); got != c.allowed {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}
