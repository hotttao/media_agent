package e2e

import (
	"net/http"
	"testing"
)

// No resources are created. The obsolete local task-pool contract must stay
// retired through the public gateway. Native execution acceptance lives in the
// independent CLI + Personal Worker capability gates.
func TestWorkflowLegacyTaskRoutesRetired(t *testing.T) {
	f := newMaterialsAPIFixture(t)
	for _, path := range []string{"/api/v1/step-runs/claim", "/api/v1/step-runs/retired-fixture/complete", "/api/v1/step-runs/retired-fixture/fail"} {
		status, _ := doJSONBearer(t, f.gateway, http.MethodPost, path, f.token, map[string]any{})
		if status != http.StatusGone {
			t.Fatalf("legacy task route %s status=%d want=410", path, status)
		}
	}
}
