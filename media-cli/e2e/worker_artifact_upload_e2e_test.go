package e2e

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

// Read-only negative gate against a completed dispatch: no signed grant,
// object upload, resource registration or Task update is permitted.
func TestWorkerArtifactUploadGatewayRejectsInvalidAdmission(t *testing.T) {
	address, userToken, workerToken := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_WORKER_API_TOKEN")
	dispatch, instance := os.Getenv("MEDIA_E2E_COMPLETED_DISPATCH_ID"), os.Getenv("MEDIA_E2E_WORKER_INSTANCE_ID")
	if address == "" || userToken == "" || workerToken == "" || dispatch == "" || instance == "" {
		t.Skip("explicit completed owned Worker dispatch required")
	}
	body := map[string]any{"worker_instance_id": instance, "attempt": 1, "candidate_index": 0, "content_type": "image/png", "size_bytes": 256, "checksum_sha256": strings.Repeat("a", 64), "content_md5": "AAAAAAAAAAAAAAAAAAAAAA=="}
	path := "/api/worker/v1/tasks/" + url.PathEscape(dispatch) + "/uploads"
	for _, token := range []string{"", userToken} {
		status, _ := doJSONBearer(t, address, http.MethodPost, path, token, body)
		if status != 401 && status != 403 {
			t.Fatalf("non-Worker upload admission status=%d", status)
		}
	}
	status, _ := doJSONBearer(t, address, http.MethodPost, path, workerToken, body)
	if status != http.StatusGone {
		t.Fatalf("completed native Task upload admission status=%d want=410", status)
	}
}
