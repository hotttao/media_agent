package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"media_agent/media_worker/biz/client/worker_gateway"
	"media_agent/media_worker/biz/dal/repo/results"
	"media_agent/media_worker/biz/workflow/execute"
)

func TestPendingResultRealProcessRestart(t *testing.T) {
	if os.Getenv("MEDIA_WORKER_TEST_SAVE_EXIT") == "1" {
		_, err := results.Open(os.Getenv("MEDIA_WORKER_TEST_STATE_DIR"), "http://test-gateway", "worker", "test-token")
		if err != nil {
			os.Exit(71)
		}
		namespace := fmt.Sprintf("%x", sha256.Sum256([]byte("http://test-gateway\x00worker\x00test-token")))
		filename := fmt.Sprintf("%x.json", sha256.Sum256([]byte("dispatch\x00stable-complete")))
		err = os.WriteFile(filepath.Join(os.Getenv("MEDIA_WORKER_TEST_STATE_DIR"), namespace, filename), []byte(`{"task":{"dispatch_id":"dispatch","attempt":1,"capability_key":"","capability_version":0,"input_json":"","lease_expires_at":0},"key":"stable-complete","output":{"candidates":[{"content":"saved result"}]}}`), 0600)
		if err != nil {
			os.Exit(72)
		}
		os.Exit(0)
	}
	dir := filepath.Join(t.TempDir(), "private")
	process := exec.Command(os.Args[0], "-test.run=^TestPendingResultRealProcessRestart$")
	process.Env = append(os.Environ(), "MEDIA_WORKER_TEST_SAVE_EXIT=1", "MEDIA_WORKER_TEST_STATE_DIR="+dir)
	if err := process.Run(); err != nil {
		t.Fatal("result persistence child failed")
	}
	store, err := results.Open(dir, "http://test-gateway", "worker", "test-token")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pending, err := store.Pending()
	if err != nil || len(pending) != 1 {
		t.Fatal("child result not recovered")
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["idempotency_key"] != "stable-complete" {
			t.Error("replay changed identity")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"receipt_id":"confirmed","status":"submitted"}`))
	}))
	defer server.Close()
	// This recovery path has no executor argument: it cannot generate again.
	if ok, err := execute.DeliverStoredResult(context.Background(), worker_gateway.New(server.URL, "test-token"), "worker", store, pending[0]); err != nil || !ok {
		t.Fatal("replay not acknowledged")
	}
	remaining, err := store.Pending()
	if err != nil || len(remaining) != 0 || calls != 1 {
		t.Fatal("confirmed pending result not retired")
	}
}
